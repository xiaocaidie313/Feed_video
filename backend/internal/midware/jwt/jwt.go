package jwt

import (
	"errors"
	"feedvid/internal/account"
	rediscache "feedvid/internal/midware/redis"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
)

const (
	ContextAccountID = "accountID"
	ContextUsername  = "username"
)

func JWTAuth(repo *account.AccountRepo, cache *rediscache.Client) gin.HandlerFunc {
	return authorize(repo, cache, false, false)
}

// Missing credentials permit anonymous reads; supplied invalid credentials return 401.
func SoftJWTAuth(repo *account.AccountRepo, cache *rediscache.Client) gin.HandlerFunc {
	return authorize(repo, cache, true, false)
}

// Native EventSource clients can pass a query token. Session validation is shared.
func SSEAuth(repo *account.AccountRepo, cache *rediscache.Client) gin.HandlerFunc {
	return authorize(repo, cache, false, true)
}
func authorize(repo *account.AccountRepo, cache *rediscache.Client, optional, allowQuery bool) gin.HandlerFunc {
	service := account.NewAccountService(repo, cache)
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		raw := ""
		if header != "" {
			parts := strings.Fields(header)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
				return
			}
			raw = parts[1]
		} else if allowQuery {
			raw = c.Query("token")
		}
		if raw == "" {
			if optional && header == "" {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing credentials"})
			return
		}
		a, err := service.Authenticate(c.Request.Context(), raw)
		if err != nil {
			if errors.Is(err, account.ErrInvalidToken) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or revoked token"})
			} else {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "authentication unavailable"})
			}
			return
		}
		c.Set(ContextAccountID, a.ID)
		c.Set(ContextUsername, a.Username)
		c.Next()
	}
}
func GetAccountID(c *gin.Context) (uint, error) {
	value, ok := c.Get(ContextAccountID)
	id, valid := value.(uint)
	if !ok || !valid || id == 0 {
		return 0, errors.New("accountID missing or invalid")
	}
	return id, nil
}
func GetUsername(c *gin.Context) (string, error) {
	value, ok := c.Get(ContextUsername)
	name, valid := value.(string)
	if !ok || !valid || name == "" {
		return "", errors.New("username missing or invalid")
	}
	return name, nil
}
