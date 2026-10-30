package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"log"
	"os"
	"sync"
	"time"
)

var secretOnce sync.Once
var cachedSecret []byte

const AccessTokenLifetime = 24 * time.Hour

func InitSecret() []byte {
	secretOnce.Do(func() {
		secret := os.Getenv("JWT_SECRET")
		if secret == "" {
			var err error
			secret, err = GenerateRefreshToken()
			if err != nil {
				panic(err)
			}
			log.Print("JWT_SECRET is unset; using an ephemeral development secret")
		}
		cachedSecret = []byte(secret)
	})
	return cachedSecret
}

type Claims struct {
	AccountID uint   `json:"account_id"`
	UserName  string `json:"username"`
	jwt.RegisteredClaims
}

func GenerateToken(id uint, username string) string {
	jti, err := GenerateRefreshToken()
	if err != nil {
		panic(err)
	}
	now := time.Now().UTC()
	claims := Claims{AccountID: id, UserName: username, RegisteredClaims: jwt.RegisteredClaims{
		ID: jti, Subject: "auth", IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenLifetime)),
	}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(InitSecret())
	if err != nil {
		panic(err)
	}
	return token
}
func GenerateRefreshToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func ParseToken(raw string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(raw, &Claims{}, func(*jwt.Token) (interface{}, error) { return InitSecret(), nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.AccountID == 0 {
		return nil, errors.New("invalid token claims")
	}
	return claims, nil
}
