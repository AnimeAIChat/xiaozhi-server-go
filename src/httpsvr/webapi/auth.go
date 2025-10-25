package webapi

import (
	"fmt"
	"time"
	"xiaozhi-server-go/src/configs"
	"xiaozhi-server-go/src/configs/database"
	"xiaozhi-server-go/src/core/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
)

type JWTClaims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

var jwtSecret = []byte("xiaozhi_jwt_secret")

// 通用认证中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		apikey := c.GetHeader("AuthorToken")
		if apikey != "" {
			// 如果提供了API Token，直接验证
			if apikey != configs.Cfg.Server.Token {
				utils.DefaultLogger.Error("无效的API Token %s", apikey)
			} else {
				utils.DefaultLogger.Info("API Token验证通过，但未设置OpenID， 校验user_id和username")
				// 检查是否设置了user_id和username
				userID, exists := c.Get("user_id")
				if !exists {
					utils.DefaultLogger.Error("API Token验证通过，但未设置user_id")
					c.JSON(401, gin.H{"status": "error", "message": "需要设置user_id"})
					c.Abort()
					return
				}
				username, exists := c.Get("username")
				if !exists {
					utils.DefaultLogger.Error("API Token验证通过，但未设置username")
					c.JSON(401, gin.H{"status": "error", "message": "需要设置username"})
					c.Abort()
					return
				}
				// 校验user_id和username是否匹配
				user, err := database.GetUserByID(database.GetDB(), userID.(uint))
				if err != nil || user == nil || user.Username != username.(string) {
					utils.DefaultLogger.Error("API Token验证通过，但user_id和username不匹配")
					c.JSON(401, gin.H{"status": "error", "message": "user_id和username不匹配"})
					c.Abort()
					return
				}
				// 认证通过，设置上下文
				c.Set("user_id", userID)
				c.Set("username", username)
				c.Next()
				return
			}
		}

		token := c.GetHeader("Authorization")
		if token == "" {
			utils.DefaultLogger.Error("未提供认证token")
			c.JSON(401, gin.H{"status": "error", "message": "未提供认证token"})
			c.Abort()
			return
		}
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}
		claims, err := VerifyJWT(token)
		if err != nil {
			utils.DefaultLogger.Error("无效的token: %v", err)
			c.JSON(401, gin.H{"status": "error", "message": "无效的token"})
			c.Abort()
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}

// 管理员权限中间件
// 允许 observer 角色对 GET 请求只读访问，非 GET 请求仍只允许 admin
func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 假设 user_id 已由认证中间件写入
		userID, exists := c.Get("user_id")
		if !exists {
			fmt.Println("未认证")
			c.JSON(401, gin.H{"status": "error", "message": "未认证"})
			c.Abort()
			return
		}
		user, err := database.GetUserByID(database.GetDB(), userID.(uint))
		if err != nil || user == nil {
			fmt.Println("权限不足，未找到用户信息")
			c.JSON(403, gin.H{"status": "error", "message": "权限不足"})
			c.Abort()
			return
		}
		// 允许只读观察员访问 GET 请求，但非 GET 方法仍需管理员权限
		method := c.Request.Method
		if method == "GET" {
			if user.Role == "admin" || user.Role == "observer" {
				c.Next()
				return
			}
			fmt.Println("权限不足，必须是管理员或观察员账号（只读）")
			c.JSON(403, gin.H{"status": "error", "message": "请使用管理员或观察员账号进行查看操作"})
			c.Abort()
			return
		}
		// 非 GET 方法仅允许 admin
		if user.Role != "admin" {
			fmt.Println("权限不足，必须是管理员账号")
			c.JSON(403, gin.H{"status": "error", "message": "请使用管理员账号进行操作"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// 生成JWT
func GenerateJWT(userID uint, username string) (string, error) {
	claims := JWTClaims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// 验证JWT
func VerifyJWT(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&JWTClaims{},
		func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		},
	)
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrInvalidKey
}
