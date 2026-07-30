package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"time"

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
		// 十六进制转化成字符串 不丢失信息  直接string 转换会变成ascll码解析 有的组合不能解析会丢失信息
		secret = hex.EncodeToString(b)
		log.Println("WARNING: JWT_SECRET is not set, generated a random one")

	}
	cachedSecret = []byte(secret)
	return cachedSecret
}

type Claims struct {
	AccountID uint   `json:"account_id"`
	UserName  string `json:"username"`
	*jwt.RegisteredClaims
}

// 生成token  添加claims
func GenerateToken(id uint, username string) string {

	now := time.Now()
	// token 可以存业务信息
	claims := Claims{
		AccountID: id,
		UserName:  username,
		RegisteredClaims: &jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour * 24)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Subject:   "auth",
		},
	}

	unsignedToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// 秘钥加密 生成token  秘钥负责签名
	tokenString, err := unsignedToken.SignedString(InitSecret())
	if err != nil {
		log.Printf("FATAL: cannot generate JWT token: %v", err)
		panic(err)
	}
	return tokenString
}

// 用于提示重新派发accessToken的token
func GenerateRefreshToken() (string, error) {
	var b = make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
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
