package auth

import (
	"os"
	"github.com/golang-jwt/jwt/v5"
)

var cachedSecret []byte

// jwt json web token

// header.payload.signature  头部.负载.签名
//
//	三个json  通过 base64 / base64url 转换成字符串 然后拼接成一个字符串 然后通过密钥进行签名
func initSecret() []byte {
	if cachedSecret != nil {
		return cachedSecret
	}

	secret := os.Getenv("JWT_SECRET")
}
