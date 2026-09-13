package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	vault "github.com/hashicorp/vault/api"
)

// VaultConfig holds Vault client configuration
type VaultConfig struct {
	Address     string
	Token       string
	RoleID      string
	SecretID    string
	Namespace   string
	MountPath   string
	AuthMethod  string
	TLSEnabled  bool
	CACert      string
	ClientCert  string
	ClientKey   string
	MaxRetries  int
	Timeout     time.Duration
}

// VaultClient wraps the Vault API client with NPDMS-specific functionality
type VaultClient struct {
	client     *vault.Client
	config     *VaultConfig
	authMethod string
}

// NewVaultConfig creates a new Vault configuration from environment variables
func NewVaultConfig() *VaultConfig {
	return &VaultConfig{
		Address:    getEnv("VAULT_ADDR", "http://localhost:8200"),
		Token:      os.Getenv("VAULT_TOKEN"),
		RoleID:     os.Getenv("VAULT_ROLE_ID"),
		SecretID:   os.Getenv("VAULT_SECRET_ID"),
		Namespace:  os.Getenv("VAULT_NAMESPACE"),
		MountPath:  getEnv("VAULT_MOUNT_PATH", "secret"),
		AuthMethod: getEnv("VAULT_AUTH_METHOD", "token"), // token, approle, kubernetes
		TLSEnabled: os.Getenv("VAULT_TLS_ENABLED") == "true",
		CACert:     os.Getenv("VAULT_CACERT"),
		ClientCert: os.Getenv("VAULT_CLIENT_CERT"),
		ClientKey:  os.Getenv("VAULT_CLIENT_KEY"),
		MaxRetries: 3,
		Timeout:    30 * time.Second,
	}
}

// NewVaultClient creates a new Vault client
func NewVaultClient(config *VaultConfig) (*VaultClient, error) {
	vaultConfig := vault.DefaultConfig()
	vaultConfig.Address = config.Address
	vaultConfig.Timeout = config.Timeout
	vaultConfig.MaxRetries = config.MaxRetries

	// Configure TLS if enabled
	if config.TLSEnabled {
		tlsConfig := &vault.TLSConfig{
			CACert:     config.CACert,
			ClientCert: config.ClientCert,
			ClientKey:  config.ClientKey,
		}
		if err := vaultConfig.ConfigureTLS(tlsConfig); err != nil {
			return nil, fmt.Errorf("failed to configure TLS: %w", err)
		}
	}

	client, err := vault.NewClient(vaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Vault client: %w", err)
	}

	// Set namespace if specified
	if config.Namespace != "" {
		client.SetNamespace(config.Namespace)
	}

	vc := &VaultClient{
		client:     client,
		config:     config,
		authMethod: config.AuthMethod,
	}

	// Authenticate based on method
	if err := vc.authenticate(); err != nil {
		return nil, fmt.Errorf("failed to authenticate with Vault: %w", err)
	}

	return vc, nil
}

// authenticate handles different authentication methods
func (vc *VaultClient) authenticate() error {
	switch vc.authMethod {
	case "token":
		if vc.config.Token == "" {
			return fmt.Errorf("VAULT_TOKEN is required for token auth")
		}
		vc.client.SetToken(vc.config.Token)
		return nil

	case "approle":
		return vc.authenticateAppRole()

	case "kubernetes":
		return vc.authenticateKubernetes()

	default:
		return fmt.Errorf("unsupported auth method: %s", vc.authMethod)
	}
}

// authenticateAppRole authenticates using AppRole
func (vc *VaultClient) authenticateAppRole() error {
	if vc.config.RoleID == "" || vc.config.SecretID == "" {
		return fmt.Errorf("VAULT_ROLE_ID and VAULT_SECRET_ID are required for AppRole auth")
	}

	data := map[string]interface{}{
		"role_id":   vc.config.RoleID,
		"secret_id": vc.config.SecretID,
	}

	resp, err := vc.client.Logical().Write("auth/approle/login", data)
	if err != nil {
		return fmt.Errorf("AppRole authentication failed: %w", err)
	}

	if resp.Auth == nil {
		return fmt.Errorf("no auth info returned from AppRole login")
	}

	vc.client.SetToken(resp.Auth.ClientToken)
	return nil
}

// authenticateKubernetes authenticates using Kubernetes service account
func (vc *VaultClient) authenticateKubernetes() error {
	// Read service account token
	jwt, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return fmt.Errorf("failed to read Kubernetes service account token: %w", err)
	}

	role := os.Getenv("VAULT_K8S_ROLE")
	if role == "" {
		role = "npdms-api"
	}

	data := map[string]interface{}{
		"role": role,
		"jwt":  string(jwt),
	}

	resp, err := vc.client.Logical().Write("auth/kubernetes/login", data)
	if err != nil {
		return fmt.Errorf("Kubernetes authentication failed: %w", err)
	}

	if resp.Auth == nil {
		return fmt.Errorf("no auth info returned from Kubernetes login")
	}

	vc.client.SetToken(resp.Auth.ClientToken)
	return nil
}

// GetSecret retrieves a secret from Vault
func (vc *VaultClient) GetSecret(ctx context.Context, path string) (map[string]interface{}, error) {
	fullPath := fmt.Sprintf("%s/data/%s", vc.config.MountPath, path)

	secret, err := vc.client.Logical().ReadWithContext(ctx, fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret at %s: %w", path, err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("secret not found at %s", path)
	}

	// KV v2 returns data nested under "data" key
	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid secret format at %s", path)
	}

	return data, nil
}

// GetSecretValue retrieves a specific key from a secret
func (vc *VaultClient) GetSecretValue(ctx context.Context, path, key string) (string, error) {
	data, err := vc.GetSecret(ctx, path)
	if err != nil {
		return "", err
	}

	value, ok := data[key].(string)
	if !ok {
		return "", fmt.Errorf("key %s not found in secret %s", key, path)
	}

	return value, nil
}

// PutSecret stores a secret in Vault
func (vc *VaultClient) PutSecret(ctx context.Context, path string, data map[string]interface{}) error {
	fullPath := fmt.Sprintf("%s/data/%s", vc.config.MountPath, path)

	payload := map[string]interface{}{
		"data": data,
	}

	_, err := vc.client.Logical().WriteWithContext(ctx, fullPath, payload)
	if err != nil {
		return fmt.Errorf("failed to write secret at %s: %w", path, err)
	}

	return nil
}

// DeleteSecret deletes a secret from Vault
func (vc *VaultClient) DeleteSecret(ctx context.Context, path string) error {
	fullPath := fmt.Sprintf("%s/data/%s", vc.config.MountPath, path)

	_, err := vc.client.Logical().DeleteWithContext(ctx, fullPath)
	if err != nil {
		return fmt.Errorf("failed to delete secret at %s: %w", path, err)
	}

	return nil
}

// GetDatabaseCredentials retrieves dynamic database credentials
func (vc *VaultClient) GetDatabaseCredentials(ctx context.Context, role string) (*DatabaseCredentials, error) {
	path := fmt.Sprintf("database/creds/%s", role)

	secret, err := vc.client.Logical().ReadWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get database credentials: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("no credentials returned for role %s", role)
	}

	creds := &DatabaseCredentials{
		Username:  secret.Data["username"].(string),
		Password:  secret.Data["password"].(string),
		LeaseID:   secret.LeaseID,
		LeaseTTL:  time.Duration(secret.LeaseDuration) * time.Second,
		Renewable: secret.Renewable,
	}

	return creds, nil
}

// DatabaseCredentials holds dynamic database credentials
type DatabaseCredentials struct {
	Username  string
	Password  string
	LeaseID   string
	LeaseTTL  time.Duration
	Renewable bool
}

// RenewLease renews a Vault lease
func (vc *VaultClient) RenewLease(ctx context.Context, leaseID string, increment int) error {
	_, err := vc.client.Sys().RenewWithContext(ctx, leaseID, increment)
	if err != nil {
		return fmt.Errorf("failed to renew lease: %w", err)
	}
	return nil
}

// RevokeLease revokes a Vault lease
func (vc *VaultClient) RevokeLease(ctx context.Context, leaseID string) error {
	err := vc.client.Sys().RevokeWithContext(ctx, leaseID)
	if err != nil {
		return fmt.Errorf("failed to revoke lease: %w", err)
	}
	return nil
}

// Encrypt encrypts data using Vault's Transit secrets engine
func (vc *VaultClient) Encrypt(ctx context.Context, keyName string, plaintext []byte) (string, error) {
	path := fmt.Sprintf("transit/encrypt/%s", keyName)

	data := map[string]interface{}{
		"plaintext": plaintext,
	}

	secret, err := vc.client.Logical().WriteWithContext(ctx, path, data)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt data: %w", err)
	}

	ciphertext, ok := secret.Data["ciphertext"].(string)
	if !ok {
		return "", fmt.Errorf("no ciphertext in response")
	}

	return ciphertext, nil
}

// Decrypt decrypts data using Vault's Transit secrets engine
func (vc *VaultClient) Decrypt(ctx context.Context, keyName, ciphertext string) ([]byte, error) {
	path := fmt.Sprintf("transit/decrypt/%s", keyName)

	data := map[string]interface{}{
		"ciphertext": ciphertext,
	}

	secret, err := vc.client.Logical().WriteWithContext(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	plaintext, ok := secret.Data["plaintext"].(string)
	if !ok {
		return nil, fmt.Errorf("no plaintext in response")
	}

	return []byte(plaintext), nil
}

// GenerateDataKey generates a new data key for envelope encryption
func (vc *VaultClient) GenerateDataKey(ctx context.Context, keyName string) (*DataKey, error) {
	path := fmt.Sprintf("transit/datakey/plaintext/%s", keyName)

	secret, err := vc.client.Logical().WriteWithContext(ctx, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate data key: %w", err)
	}

	return &DataKey{
		Plaintext:  secret.Data["plaintext"].(string),
		Ciphertext: secret.Data["ciphertext"].(string),
	}, nil
}

// DataKey holds a generated data key
type DataKey struct {
	Plaintext  string // Base64 encoded plaintext key
	Ciphertext string // Encrypted key (safe to store)
}

// RotateEncryptionKey rotates the encryption key in Transit
func (vc *VaultClient) RotateEncryptionKey(ctx context.Context, keyName string) error {
	path := fmt.Sprintf("transit/keys/%s/rotate", keyName)

	_, err := vc.client.Logical().WriteWithContext(ctx, path, nil)
	if err != nil {
		return fmt.Errorf("failed to rotate key: %w", err)
	}

	return nil
}

// Health checks Vault health status
func (vc *VaultClient) Health(ctx context.Context) (*VaultHealth, error) {
	health, err := vc.client.Sys().HealthWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check Vault health: %w", err)
	}

	return &VaultHealth{
		Initialized:   health.Initialized,
		Sealed:        health.Sealed,
		Standby:       health.Standby,
		ServerTimeUTC: health.ServerTimeUTC,
		Version:       health.Version,
		ClusterName:   health.ClusterName,
	}, nil
}

// VaultHealth holds Vault health information
type VaultHealth struct {
	Initialized   bool
	Sealed        bool
	Standby       bool
	ServerTimeUTC int64
	Version       string
	ClusterName   string
}

// TokenInfo holds token information
type TokenInfo struct {
	Accessor     string
	CreationTime int64
	TTL          int64
	Renewable    bool
	Policies     []string
}

// GetTokenInfo retrieves information about the current token
func (vc *VaultClient) GetTokenInfo(ctx context.Context) (*TokenInfo, error) {
	secret, err := vc.client.Auth().Token().LookupSelfWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup token: %w", err)
	}

	info := &TokenInfo{}

	if v, ok := secret.Data["accessor"].(string); ok {
		info.Accessor = v
	}
	if v, ok := secret.Data["creation_time"].(json.Number); ok {
		info.CreationTime, _ = v.Int64()
	}
	if v, ok := secret.Data["ttl"].(json.Number); ok {
		info.TTL, _ = v.Int64()
	}
	if v, ok := secret.Data["renewable"].(bool); ok {
		info.Renewable = v
	}
	if v, ok := secret.Data["policies"].([]interface{}); ok {
		for _, p := range v {
			if policy, ok := p.(string); ok {
				info.Policies = append(info.Policies, policy)
			}
		}
	}

	return info, nil
}

// RenewToken renews the current token
func (vc *VaultClient) RenewToken(ctx context.Context) error {
	_, err := vc.client.Auth().Token().RenewSelfWithContext(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to renew token: %w", err)
	}
	return nil
}

// StartTokenRenewer starts a background token renewer
func (vc *VaultClient) StartTokenRenewer(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := vc.RenewToken(ctx); err != nil {
					log.Printf("Failed to renew Vault token: %v", err)
				}
			}
		}
	}()
}

// Close closes the Vault client
func (vc *VaultClient) Close() {
	// Clear the token
	vc.client.ClearToken()
}

// getEnv is defined in config.go
