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

// ErrAccountClosed is returned when the account behind a token no longer signs
// in. The handler turns it into a 401 saying so, rather than the generic
// "invalid refresh token", because an officer whose account was closed while
// they were working deserves to be told which of the two happened.
var ErrAccountClosed = errors.New("this account has been deactivated")

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*models.LoginResponse, error) {
	// Validate refresh token
	token, err := jwt.Parse(refreshToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(s.jwtSecret), nil
	})

	if err != nil || !token.Valid {
		return nil, errors.New("invalid refresh token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}

	// It must be a refresh token, not an access token.
	//
	// Both are signed with the same secret, so without this check either one
	// is accepted here — and an access token presented to /auth/refresh would
	// be exchanged for a fresh pair, which turns a one-hour credential into a
	// permanent one. The `type` claim has always been minted; nothing read it.
	if tokenType, _ := claims["type"].(string); tokenType != "refresh" {
		return nil, errors.New("invalid refresh token")
	}

	// A missing or non-string claim is a token this service did not mint.
	// Asserting without the comma-ok panics the request instead of refusing it.
	subject, ok := claims["userId"].(string)
	if !ok {
		return nil, errors.New("invalid token claims")
	}
	userID, err := uuid.Parse(subject)
	if err != nil {
		return nil, errors.New("invalid user ID")
	}

	// Get user
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// The account must still be one that signs in.
	//
	// Login refuses a deactivated officer — FindByUsername filters on
	// is_active — but this path loads by id and did not check, so an officer
	// who was deactivated while holding a refresh token could exchange it for
	// a new pair indefinitely: every refresh mints a further seven days, so
	// the credential never expired and the officer never had to log in again
	// to be refused. Deactivation, the whole of migration 000086, stopped at
	// the login screen and did not reach anyone already through it.
	if !user.IsActive {
		return nil, ErrAccountClosed
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

	// Drop the session from the user's active set. Without this the
	// concurrent-session cap counts sign-ins that have already ended, and the
	// user is eventually refused a new one.
	bearer := strings.TrimSpace(strings.TrimPrefix(token, "Bearer "))

	// Blacklist the token so it cannot be replayed before it expires.
	//
	// Keyed on the bare token, which is what AuthMiddleware holds and looks
	// up. It was keyed on the whole Authorization header — "Bearer eyJ..." —
	// so even once the check existed the two would never have met.
	if bearer != "" {
		s.redis.Set(ctx, "blacklist:"+bearer, "1", time.Hour)
	}

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

// UserIDFromRefreshToken names the account a refused refresh was made for, so
// the refusal can be recorded against it. The token's signature has already
// been checked by the caller; nothing here grants anything.
func (s *AuthService) UserIDFromRefreshToken(token string) *uuid.UUID {
	claims, err := s.parseClaims(strings.TrimSpace(strings.TrimPrefix(token, "Bearer ")))
	if err != nil {
		return nil
	}
	subject, ok := claims["userId"].(string)
	if !ok {
		return nil
	}
	userID, err := uuid.Parse(subject)
	if err != nil {
		return nil
	}
	return &userID
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
