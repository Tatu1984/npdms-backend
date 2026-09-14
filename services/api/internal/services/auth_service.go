package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/npdms/api/internal/models"
	"github.com/npdms/api/internal/repository"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	userRepo  *repository.UserRepository
	redis     *redis.Client
	jwtSecret string
}

func NewAuthService(userRepo *repository.UserRepository, redis *redis.Client, jwtSecret string) *AuthService {
	return &AuthService{
		userRepo:  userRepo,
		redis:     redis,
		jwtSecret: jwtSecret,
	}
}

func (s *AuthService) Login(ctx context.Context, username, password string) (*models.LoginResponse, error) {
	// Find user by username
	user, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}

	// Update last login
	s.userRepo.UpdateLastLogin(ctx, user.ID)

	// Generate tokens
	accessToken, err := s.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.generateRefreshToken(user.ID)
	if err != nil {
		return nil, err
	}

	return &models.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    3600, // 1 hour
		User:         *user,
	}, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*models.LoginResponse, error) {
	// Validate refresh token
	token, err := jwt.Parse(refreshToken, func(token *jwt.Token) (interface{}, error) {
		return []byte(s.jwtSecret), nil
	})

	if err != nil || !token.Valid {
		return nil, errors.New("invalid refresh token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}

	userID, err := uuid.Parse(claims["userId"].(string))
	if err != nil {
		return nil, errors.New("invalid user ID")
	}

	// Get user
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Generate new tokens
	accessToken, err := s.generateAccessToken(user)
	if err != nil {
		return nil, err
	}

	newRefreshToken, err := s.generateRefreshToken(user.ID)
	if err != nil {
		return nil, err
	}

	return &models.LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresIn:    3600,
		User:         *user,
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, userID uuid.UUID, token string) error {
	if s.redis == nil {
		return nil
	}

	// Blacklist the token so it cannot be replayed before it expires.
	s.redis.Set(ctx, "blacklist:"+token, "1", time.Hour)

	// Drop the session from the user's active set. Without this the
	// concurrent-session cap counts sign-ins that have already ended, and the
	// user is eventually refused a new one.
	bearer := strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))

	// /auth/logout is a public route, so the caller is not resolved by
	// AuthMiddleware and userID arrives as the zero UUID. Recover it from the
	// token's own claims, otherwise the session is removed from the wrong key
	// and the user's active-session count never falls.
	if userID == uuid.Nil && bearer != "" {
		if claims, err := s.parseClaims(bearer); err == nil {
			if sub, ok := claims["userId"].(string); ok {
				if parsed, err := uuid.Parse(sub); err == nil {
					userID = parsed
				}
			}
		}
	}

	if bearer != "" && userID != uuid.Nil {
		sum := sha256.Sum256([]byte(bearer))
		sessionToken := hex.EncodeToString(sum[:16])
		s.redis.SRem(ctx, fmt.Sprintf("user_sessions:%s", userID), sessionToken)
		s.redis.Del(ctx, fmt.Sprintf("session:%s:%s", userID, sessionToken))
	}
	return nil
}

// parseClaims reads a token's claims without requiring it to be unexpired:
// logging out with a just-expired token must still release the session.
func (s *AuthService) parseClaims(token string) (jwt.MapClaims, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		return []byte(s.jwtSecret), nil
	}, jwt.WithoutClaimsValidation())
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("unexpected claims type")
	}
	return claims, nil
}

func (s *AuthService) GetUser(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	return s.userRepo.FindByID(ctx, userID)
}

func (s *AuthService) UpdateProfile(ctx context.Context, userID uuid.UUID, name, email, phone string) error {
	return s.userRepo.UpdateProfile(ctx, userID, name, email, phone)
}

func (s *AuthService) GetUserWithStation(ctx context.Context, userID uuid.UUID) (*models.User, string, error) {
	return s.userRepo.GetUserWithStation(ctx, userID)
}

func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, oldPassword, newPassword string) error {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}

	// Verify old password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return errors.New("incorrect current password")
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.userRepo.UpdatePassword(ctx, userID, string(hash))
}

func (s *AuthService) generateAccessToken(user *models.User) (string, error) {
	claims := jwt.MapClaims{
		"userId":    user.ID.String(),
		"username":  user.Username,
		"role":      string(user.Role),
		"stationId": "",
		"exp":       time.Now().Add(time.Hour).Unix(),
		"iat":       time.Now().Unix(),
	}

	if user.StationID != nil {
		claims["stationId"] = user.StationID.String()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

func (s *AuthService) generateRefreshToken(userID uuid.UUID) (string, error) {
	claims := jwt.MapClaims{
		"userId": userID.String(),
		"exp":    time.Now().Add(7 * 24 * time.Hour).Unix(), // 7 days
		"iat":    time.Now().Unix(),
		"type":   "refresh",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

// HashPassword hashes a password for storage
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}
