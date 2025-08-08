package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AuthToken struct {
	secretKey []byte
}

func NewAuthToken(secretKey string) *AuthToken {
	// 添加验证，确保密钥不为空
	if secretKey == "" {
		fmt.Println("Error! secret key cannot be empty")
	}
	return &AuthToken{
		secretKey: []byte(secretKey),
	}
}

func (at *AuthToken) GenerateToken(deviceID string) (string, error) {
	// 设置过期时间为1小时后
	expireTime := time.Now().Add(time.Hour)

	// 创建claims
	claims := jwt.MapClaims{
		"device_id": deviceID,
		"exp":       expireTime.Unix(),
		"iat":       time.Now().Unix(), // 添加签发时间
	}

	// 创建token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 使用密钥签名
	tokenString, err := token.SignedString(at.secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

func (at *AuthToken) VerifyToken(tokenString string) (bool, string, error) {
	if at == nil {
		return false, "", errors.New("AuthToken instance is nil")
	}

	if at.secretKey == nil {
		return false, "", errors.New("secret key is not initialized")
	}

	// 解析token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return at.secretKey, nil
	})
	if err != nil {
		return false, "", fmt.Errorf("failed to parse token: %w", err)
	}

	// 验证token是否有效
	if !token.Valid {
		return false, "", errors.New("invalid token")
	}

	// 获取claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return false, "", errors.New("invalid claims")
	}

	// 获取设备ID
	deviceID, ok := claims["device_id"].(string)
	if !ok {
		return false, "", errors.New("invalid device_id in claims")
	}

	return true, deviceID, nil
}
