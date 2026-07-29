package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"time"
	"uuid"

	"github.com/golang-jwt/jwt/v5"
)

var cachedSecret []byte

// jwt json web token

// header.payload.signature  头部.负载.签名
//
//	三个json  通过 base64 / base64url 转换成字符串 然后拼接成一个字符串 然后通过密钥进行签名
func InitSecret() []byte {
	if cachedSecret != nil {
		return cachedSecret
	}

	secret := os.Getenv("JWT_SECRET")

	if secret == "" {
		b := make([]byte, 32)
		_, err := rand.Read(b)
		if err != nil {
			log.Printf("FATAL: cannot generate JWT secret: %v", err)
			panic(err)
		}
		secret = hex.EncodeToString(b)
		log.Println("WARNING: JWT_SECRET is not set, generated a random one")

	}
	cachedSecret = []byte(secret)
	return cachedSecret
}

type Claims struct {
	AccountID string `json:"account_id"`
	UserName  string `json:"username"`
	*jwt.RegisteredClaims
}

// 生成token  添加claims
func GenerateToken() string {

	now := time.Now()
	claims := Claims{
		AccountID: uuid.New().String(),
		UserName:  "test",
		RegisteredClaims: &jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour * 24)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Subject:   "auth",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(InitSecret())
	if err != nil {
		log.Printf("FATAL: cannot generate JWT token: %v", err)
		panic(err)
	}
	return tokenString
}

func InitRefreshSecret() (string, error) {
	b := make([]byte, 32)

	if _, err := rand.Read(b); err != nil {
		log.Printf("FATAL: cannot generate refresh JWT secret: %v", err)
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func ParseToken(tokenstring string) (*Claims, error) {
	// 解析token 并验证签名 // 合并的字符串形式 ——-》解析成 Header.claims.signature形式
	token, err := jwt.ParseWithClaims(tokenstring, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method == nil || token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return InitSecret(), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("invalid token claims")
	}
	return claims, nil
}
