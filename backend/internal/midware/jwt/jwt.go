package jwt

import (
	"context"
	"errors"
	"feedvid/internal/account"
	"feedvid/internal/auth"
	rediscache "feedvid/internal/midware/redis"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	ContextAccountID = "accountID"
	ContextUsername  = "username"
)

// JWTAuth 校验 Bearer JWT，通过后把用户信息写入 gin.Context。
func JWTAuth(accountRepo *account.AccountRepo, cache *rediscache.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
			return
		}

		tokenString := parts[1]

		claims, err := auth.ParseToken(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}
		check(c, claims, tokenString, accountRepo, cache)
	}
}

// SoftJWTAuth 可选鉴权：有合法 token 则写入 accountID，否则继续匿名访问。
func SoftJWTAuth(accountRepo *account.AccountRepo, cache *rediscache.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
			return
		}

		tokenString := parts[1]

		claims, err := auth.ParseToken(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		check(c, claims, tokenString, accountRepo, cache)
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

func check(c *gin.Context, claims *auth.Claims, tokenString string, accountRepo *account.AccountRepo, cache *rediscache.Client) (bool, error) {
	// 先查redis

	if cache != nil {
		// 创建查询key -》 存放token
		key := cache.Key("accountID:%d", claims.AccountID)
		cacheCtx, cancel := context.WithTimeout(c.Request.Context(), 50*time.Millisecond)
		defer cancel()

		b, err := cache.GetBytes(cacheCtx, key)
		if err == nil {
			if string(b) != tokenString {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token has been revoked"})
				return false, nil
			}
			c.Set("accountID", claims.AccountID)
			c.Set("username", claims.UserName)
			c.Next()
			return true, nil
		}
	}
	// redis 没找到 查db

	accountInfo, err := accountRepo.FindUserByAccountID(c.Request.Context(), claims.AccountID)
	if err != nil || accountInfo.Token == "" || accountInfo.Token != tokenString {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token has been revoked"})
		return false, err
	}
	if cache != nil {
		key := cache.Key("accountID:%d", claims.AccountID)
		cacheCtx, cancle := context.WithTimeout(c.Request.Context(), 50*time.Millisecond)
		defer cancle()
		cache.SetBytes(cacheCtx, key, []byte(tokenString), 0)

	}
	c.Set("accountID", claims.AccountID)
	c.Set("username", claims.UserName)
	c.Next()
	return true, nil
}
