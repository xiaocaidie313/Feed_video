package account

import (
	"context"
	"errors"
	"feedvid/internal/apierror"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"testing"
)

func TestPasswordVerification(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassword(context.Background(), string(hash), "correct-password"); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassword(context.Background(), string(hash), "wrong-password"); err == nil {
		t.Fatal("wrong password accepted")
	}
}
func TestRegistrationRejectsInvalidInputBeforeDatabaseAccess(t *testing.T) {
	service := NewAccountService(nil, nil)
	for _, a := range []*Account{nil, {Username: "ab", Password: "password123"}, {Username: "alice", Password: "short"}, {Username: "alice", Password: strings.Repeat("a", 73)}, {Username: "alice", Password: strings.Repeat("密", 25)}} {
		if err := service.CreateAccount(context.Background(), a); !errors.Is(err, apierror.ErrValidation) {
			t.Fatalf("expected validation error, got %v", err)
		}
	}
}
