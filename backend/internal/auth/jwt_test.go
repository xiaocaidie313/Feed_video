package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"sync"
	"testing"
	"time"
)

func TestAccessTokensAreUniqueAndParseable(t *testing.T) {
	first, second := GenerateToken(42, "alice"), GenerateToken(42, "alice")
	if first == second {
		t.Fatal("same-second login and refresh must issue distinct tokens")
	}
	claims, err := ParseToken(first)
	if err != nil || claims.AccountID != 42 || claims.ID == "" {
		t.Fatalf("invalid generated token: %v", err)
	}
}
func TestRejectInvalidTokens(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method jwt.SigningMethod
		claims jwt.MapClaims
	}{
		{"missing expiry", jwt.SigningMethodHS256, jwt.MapClaims{"account_id": 1}},
		{"expired", jwt.SigningMethodHS256, jwt.MapClaims{"account_id": 1, "exp": time.Now().Add(-time.Hour).Unix()}},
		{"wrong algorithm", jwt.SigningMethodHS384, jwt.MapClaims{"account_id": 1, "exp": time.Now().Add(time.Hour).Unix()}},
		{"missing identity", jwt.SigningMethodHS256, jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := jwt.NewWithClaims(tc.method, tc.claims).SignedString(InitSecret())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseToken(raw); err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
}
func TestConcurrentTokenGeneration(t *testing.T) {
	var wg sync.WaitGroup
	tokens := make(chan string, 32)
	for range 32 {
		wg.Add(1)
		go func() { defer wg.Done(); tokens <- GenerateToken(1, "alice") }()
	}
	wg.Wait()
	close(tokens)
	seen := map[string]bool{}
	for raw := range tokens {
		if seen[raw] {
			t.Fatal("duplicate token")
		}
		seen[raw] = true
	}
}
