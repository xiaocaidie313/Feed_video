package jwt

import (
	"errors"
	"net/http"
	"strings"

	"feedvid/internal/account"
	"feedvid/internal/auth"

	"github.com/gin-gonic/gin"
)

const (
	ContextAccountID = "accountID"
	ContextUsername  = "username"
)

// JWTAuth 校验 Bearer JWT，通过后把用户信息写入 gin.Context。
func JWTAuth(accountService *account.AccountService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 劫取bearer token 解析 --》获得其中的 claims
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		tokenString := parts[1] //bear的 token

		claims, err := auth.ParseToken(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		if claims.AccountID == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		acc, err := accountService.FindByID(c.Request.Context(), claims.AccountID)
		if err != nil || acc == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		// 与库中当前会话 token 比对，登出/换票后旧 JWT 失效
		if acc.Token == "" || acc.Token != tokenString {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		c.Set(ContextAccountID, claims.AccountID)
		c.Set(ContextUsername, claims.UserName)
		c.Next()
	}
}

func GetAccountID(c *gin.Context) (uint, error) {
	val, exists := c.Get(ContextAccountID)
	if !exists {
		return 0, errors.New("accountID not found")
	}
	accountID, ok := val.(uint)
	if !ok {
		return 0, errors.New("accountID has invalid type")
	}
	return accountID, nil
}

func GetUsername(c *gin.Context) (string, error) {
	val, exists := c.Get(ContextUsername)
	if !exists {
		return "", errors.New("username not found")
	}
	username, ok := val.(string)
	if !ok {
		return "", errors.New("username has invalid type")
	}
	return username, nil
}
