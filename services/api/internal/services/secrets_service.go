package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/npdms/api/internal/config"
)

// SecretsService provides centralized secrets management
type SecretsService struct {
	vault       *config.VaultClient
	cache       *secretsCache
	cacheTTL    time.Duration
	encKeyName  string
}

// secretsCache provides in-memory caching for secrets
type secretsCache struct {
	mu      sync.RWMutex
	items   map[string]*cacheItem
	maxSize int
}

type cacheItem struct {
	value     interface{}
	expiresAt time.Time
}

// NewSecretsService creates a new secrets service
func NewSecretsService(vault *config.VaultClient) *SecretsService {
	return &SecretsService{
		vault:      vault,
		cache:      newSecretsCache(1000),
		cacheTTL:   5 * time.Minute,
		encKeyName: "npdms-data-encryption",
	}
}

func newSecretsCache(maxSize int) *secretsCache {
	return &secretsCache{
		items:   make(map[string]*cacheItem),
		maxSize: maxSize,
	}
}

func (c *secretsCache) get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[key]
	if !ok || time.Now().After(item.expiresAt) {
		return nil, false
	}
	return item.value, true
}

func (c *secretsCache) set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict old entries if cache is full
	if len(c.items) >= c.maxSize {
		now := time.Now()
		for k, v := range c.items {
			if now.After(v.expiresAt) {
				delete(c.items, k)
			}
		}
	}

	c.items[key] = &cacheItem{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
}

func (c *secretsCache) delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// GetDatabaseConfig retrieves database configuration from Vault
func (s *SecretsService) GetDatabaseConfig(ctx context.Context) (*DatabaseConfig, error) {
	cacheKey := "db-config"

	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.(*DatabaseConfig), nil
	}

	data, err := s.vault.GetSecret(ctx, "npdms/database")
	if err != nil {
		return nil, fmt.Errorf("failed to get database config: %w", err)
	}

	cfg := &DatabaseConfig{
		Host:     data["host"].(string),
		Port:     int(data["port"].(float64)),
		Database: data["database"].(string),
		SSLMode:  data["sslmode"].(string),
	}

	s.cache.set(cacheKey, cfg, s.cacheTTL)
	return cfg, nil
}

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	Host     string
	Port     int
	Database string
	SSLMode  string
}

// GetDynamicDatabaseCredentials retrieves dynamic database credentials
func (s *SecretsService) GetDynamicDatabaseCredentials(ctx context.Context) (*config.DatabaseCredentials, error) {
	return s.vault.GetDatabaseCredentials(ctx, "npdms-api")
}

// GetAPIKey retrieves an API key from Vault
func (s *SecretsService) GetAPIKey(ctx context.Context, service string) (string, error) {
	cacheKey := fmt.Sprintf("api-key-%s", service)

	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.(string), nil
	}

	key, err := s.vault.GetSecretValue(ctx, fmt.Sprintf("npdms/api-keys/%s", service), "key")
	if err != nil {
		return "", fmt.Errorf("failed to get API key for %s: %w", service, err)
	}

	s.cache.set(cacheKey, key, s.cacheTTL)
	return key, nil
}

// GetJWTSecret retrieves the JWT signing secret
func (s *SecretsService) GetJWTSecret(ctx context.Context) (string, error) {
	cacheKey := "jwt-secret"

	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.(string), nil
	}

	secret, err := s.vault.GetSecretValue(ctx, "npdms/jwt", "secret")
	if err != nil {
		return "", fmt.Errorf("failed to get JWT secret: %w", err)
	}

	s.cache.set(cacheKey, secret, s.cacheTTL)
	return secret, nil
}

// GetRedisPassword retrieves the Redis password
func (s *SecretsService) GetRedisPassword(ctx context.Context) (string, error) {
	cacheKey := "redis-password"

	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.(string), nil
	}

	password, err := s.vault.GetSecretValue(ctx, "npdms/redis", "password")
	if err != nil {
		return "", fmt.Errorf("failed to get Redis password: %w", err)
	}

	s.cache.set(cacheKey, password, s.cacheTTL)
	return password, nil
}

// GetEncryptionKey retrieves an encryption key for a specific purpose
func (s *SecretsService) GetEncryptionKey(ctx context.Context, purpose string) ([]byte, error) {
	cacheKey := fmt.Sprintf("enc-key-%s", purpose)

	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.([]byte), nil
	}

	keyB64, err := s.vault.GetSecretValue(ctx, fmt.Sprintf("npdms/encryption-keys/%s", purpose), "key")
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key for %s: %w", purpose, err)
	}

	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encryption key: %w", err)
	}

	s.cache.set(cacheKey, key, s.cacheTTL)
	return key, nil
}

// EncryptSensitiveField encrypts a sensitive field using Vault Transit
func (s *SecretsService) EncryptSensitiveField(ctx context.Context, plaintext string) (string, error) {
	ciphertext, err := s.vault.Encrypt(ctx, s.encKeyName, []byte(plaintext))
	if err != nil {
		return "", fmt.Errorf("failed to encrypt field: %w", err)
	}
	return ciphertext, nil
}

// DecryptSensitiveField decrypts a sensitive field using Vault Transit
func (s *SecretsService) DecryptSensitiveField(ctx context.Context, ciphertext string) (string, error) {
	plaintext, err := s.vault.Decrypt(ctx, s.encKeyName, ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt field: %w", err)
	}
	return string(plaintext), nil
}

// EncryptAadhaarNumber encrypts an Aadhaar number using Vault Transit
func (s *SecretsService) EncryptAadhaarNumber(ctx context.Context, aadhaar string) (string, error) {
	return s.vault.Encrypt(ctx, "npdms-aadhaar-encryption", []byte(aadhaar))
}

// DecryptAadhaarNumber decrypts an Aadhaar number using Vault Transit
func (s *SecretsService) DecryptAadhaarNumber(ctx context.Context, encryptedAadhaar string) (string, error) {
	plaintext, err := s.vault.Decrypt(ctx, "npdms-aadhaar-encryption", encryptedAadhaar)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// GenerateDataEncryptionKey generates a new data encryption key
func (s *SecretsService) GenerateDataEncryptionKey(ctx context.Context) (*config.DataKey, error) {
	return s.vault.GenerateDataKey(ctx, s.encKeyName)
}

// RotateEncryptionKey rotates the encryption key
func (s *SecretsService) RotateEncryptionKey(ctx context.Context) error {
	if err := s.vault.RotateEncryptionKey(ctx, s.encKeyName); err != nil {
		return fmt.Errorf("failed to rotate encryption key: %w", err)
	}

	// Clear cached keys
	s.cache.delete(fmt.Sprintf("enc-key-%s", s.encKeyName))
	return nil
}

// GetOAuthCredentials retrieves OAuth credentials for external services
func (s *SecretsService) GetOAuthCredentials(ctx context.Context, provider string) (*OAuthCredentials, error) {
	cacheKey := fmt.Sprintf("oauth-%s", provider)

	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.(*OAuthCredentials), nil
	}

	data, err := s.vault.GetSecret(ctx, fmt.Sprintf("npdms/oauth/%s", provider))
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth credentials for %s: %w", provider, err)
	}

	creds := &OAuthCredentials{
		ClientID:     data["client_id"].(string),
		ClientSecret: data["client_secret"].(string),
	}

	if v, ok := data["redirect_uri"].(string); ok {
		creds.RedirectURI = v
	}

	s.cache.set(cacheKey, creds, s.cacheTTL)
	return creds, nil
}

// OAuthCredentials holds OAuth configuration
type OAuthCredentials struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// GetSMSCredentials retrieves SMS gateway credentials
func (s *SecretsService) GetSMSCredentials(ctx context.Context) (*SMSCredentials, error) {
	cacheKey := "sms-credentials"

	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.(*SMSCredentials), nil
	}

	data, err := s.vault.GetSecret(ctx, "npdms/sms")
	if err != nil {
		return nil, fmt.Errorf("failed to get SMS credentials: %w", err)
	}

	creds := &SMSCredentials{
		Provider: data["provider"].(string),
		APIKey:   data["api_key"].(string),
		SenderID: data["sender_id"].(string),
	}

	s.cache.set(cacheKey, creds, s.cacheTTL)
	return creds, nil
}

// SMSCredentials holds SMS gateway configuration
type SMSCredentials struct {
	Provider string
	APIKey   string
	SenderID string
}

// GetEmailCredentials retrieves email service credentials
func (s *SecretsService) GetEmailCredentials(ctx context.Context) (*EmailCredentials, error) {
	cacheKey := "email-credentials"

	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.(*EmailCredentials), nil
	}

	data, err := s.vault.GetSecret(ctx, "npdms/email")
	if err != nil {
		return nil, fmt.Errorf("failed to get email credentials: %w", err)
	}

	creds := &EmailCredentials{
		SMTPHost:     data["smtp_host"].(string),
		SMTPPort:     int(data["smtp_port"].(float64)),
		Username:     data["username"].(string),
		Password:     data["password"].(string),
		FromAddress:  data["from_address"].(string),
		FromName:     data["from_name"].(string),
	}

	s.cache.set(cacheKey, creds, s.cacheTTL)
	return creds, nil
}

// EmailCredentials holds email service configuration
type EmailCredentials struct {
	SMTPHost    string
	SMTPPort    int
	Username    string
	Password    string
	FromAddress string
	FromName    string
}

// GetPaymentGatewayCredentials retrieves payment gateway credentials
func (s *SecretsService) GetPaymentGatewayCredentials(ctx context.Context, gateway string) (*PaymentGatewayCredentials, error) {
	cacheKey := fmt.Sprintf("payment-%s", gateway)

	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.(*PaymentGatewayCredentials), nil
	}

	data, err := s.vault.GetSecret(ctx, fmt.Sprintf("npdms/payment/%s", gateway))
	if err != nil {
		return nil, fmt.Errorf("failed to get payment gateway credentials for %s: %w", gateway, err)
	}

	creds := &PaymentGatewayCredentials{
		KeyID:     data["key_id"].(string),
		KeySecret: data["key_secret"].(string),
	}

	if v, ok := data["webhook_secret"].(string); ok {
		creds.WebhookSecret = v
	}

	s.cache.set(cacheKey, creds, s.cacheTTL)
	return creds, nil
}

// PaymentGatewayCredentials holds payment gateway configuration
type PaymentGatewayCredentials struct {
	KeyID         string
	KeySecret     string
	WebhookSecret string
}

// GetS3Credentials retrieves S3/MinIO credentials
func (s *SecretsService) GetS3Credentials(ctx context.Context) (*S3Credentials, error) {
	cacheKey := "s3-credentials"

	if cached, ok := s.cache.get(cacheKey); ok {
		return cached.(*S3Credentials), nil
	}

	data, err := s.vault.GetSecret(ctx, "npdms/s3")
	if err != nil {
		return nil, fmt.Errorf("failed to get S3 credentials: %w", err)
	}

	creds := &S3Credentials{
		Endpoint:        data["endpoint"].(string),
		AccessKeyID:     data["access_key_id"].(string),
		SecretAccessKey: data["secret_access_key"].(string),
		Bucket:          data["bucket"].(string),
		Region:          data["region"].(string),
	}

	s.cache.set(cacheKey, creds, s.cacheTTL)
	return creds, nil
}

// S3Credentials holds S3 configuration
type S3Credentials struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Region          string
}

// StoreSecret stores a secret in Vault (for admin operations)
func (s *SecretsService) StoreSecret(ctx context.Context, path string, data map[string]interface{}) error {
	return s.vault.PutSecret(ctx, path, data)
}

// InvalidateCache invalidates a cached secret
func (s *SecretsService) InvalidateCache(key string) {
	s.cache.delete(key)
}

// ClearCache clears all cached secrets
func (s *SecretsService) ClearCache() {
	s.cache.mu.Lock()
	defer s.cache.mu.Unlock()
	s.cache.items = make(map[string]*cacheItem)
}

// HealthCheck checks if the secrets service is healthy
func (s *SecretsService) HealthCheck(ctx context.Context) error {
	health, err := s.vault.Health(ctx)
	if err != nil {
		return fmt.Errorf("vault health check failed: %w", err)
	}

	if health.Sealed {
		return fmt.Errorf("vault is sealed")
	}

	if !health.Initialized {
		return fmt.Errorf("vault is not initialized")
	}

	return nil
}
