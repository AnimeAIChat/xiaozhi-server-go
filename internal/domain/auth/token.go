package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// AuthToken signs and verifies device scoped JWT tokens.
type AuthToken struct {
	secretKey []byte
	ttl       time.Duration
}

// NewAuthToken builds a token helper using the provided secret.
func NewAuthToken(secretKey string) *AuthToken {
	token := &AuthToken{
		secretKey: []byte(secretKey),
		ttl:       time.Hour,
	}
	if secretKey == "" {
		fmt.Println("auth token secret key cannot be empty")
	}
	return token
}

// WithTTL allows customising the expiration duration.
func (at *AuthToken) WithTTL(ttl time.Duration) *AuthToken {
	if ttl > 0 {
		at.ttl = ttl
	}
	return at
}

// GenerateToken issues a JWT for the provided device identifier.
func (at *AuthToken) GenerateToken(deviceID string) (string, error) {
	if at == nil {
		return "", errors.New("auth token is nil")
	}
	if len(at.secretKey) == 0 {
		return "", errors.New("auth token secret is empty")
	}

	expireTime := time.Now().Add(at.ttl)
	claims := jwt.MapClaims{
		"device_id": deviceID,
		"exp":       expireTime.Unix(),
		"iat":       time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(at.secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}
	return tokenString, nil
}

// VerifyToken validates the JWT and extracts the device identifier.
func (at *AuthToken) VerifyToken(tokenString string) (bool, string, error) {
	if at == nil {
		return false, "", errors.New("auth token is nil")
	}
	if len(at.secretKey) == 0 {
		return false, "", errors.New("auth token secret is empty")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return at.secretKey, nil
	})
	if err != nil {
		return false, "", fmt.Errorf("failed to parse token: %w", err)
	}
	if !token.Valid {
		return false, "", errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return false, "", errors.New("invalid claims")
	}
	deviceID, ok := claims["device_id"].(string)
	if !ok {
		return false, "", errors.New("invalid device_id claim")
	}
	return true, deviceID, nil
}
