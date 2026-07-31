package internal

import (
	"errors"

	"github.com/gin-gonic/gin"
)

func GetUsername(c *gin.Context) (string, error) {
	val, exists := c.Get("username")
	if !exists {
		return "", errors.New("username not found")
	}
	username, ok := val.(string)
	if !ok {
		return "", errors.New("username is not a string")
	}
	return username, nil
}
