package account

import (
	"context"
	"errors"
	"feedvid/internal/apierror"
	"feedvid/internal/auth"
	rediscache "feedvid/internal/midware/redis"
	"fmt"
	mysqldriver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"log"
	"strings"
	"time"
	"unicode/utf8"
)

type AccountService struct {
	repo  *AccountRepo
	cache *rediscache.Client
}

var (
	ErrInvalidName       = errors.New("invalid username")
	ErrInvalidToken      = errors.New("invalid or revoked token")
	ErrNameAlreadyExists = errors.New("username already exists")
	ErrNameIsEmpty       = errors.New("username is empty")
	ErrInvalidUsername   = errors.New("invalid username or password")
	ErrInvalidPassword   = errors.New("invalid username or password")
)

const refreshLifetime = 7 * 24 * time.Hour

func NewAccountService(repo *AccountRepo, cache *rediscache.Client) *AccountService {
	return &AccountService{repo: repo, cache: cache}
}
func (s *AccountService) Repo() *AccountRepo {
	if s == nil {
		return nil
	}
	return s.repo
}
func (s *AccountService) Cache() *rediscache.Client {
	if s == nil {
		return nil
	}
	return s.cache
}
func VerifyPassword(ctx context.Context, hash, password string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}
func validateUsername(name string) error {
	n := utf8.RuneCountInString(name)
	if n < 3 || n > 32 {
		return fmt.Errorf("username must contain 3 to 32 characters: %w", apierror.ErrValidation)
	}
	return nil
}
func validatePassword(password string) error {
	if len(password) < 8 || len(password) > 72 {
		return fmt.Errorf("password must contain 8 to 72 bytes: %w", apierror.ErrValidation)
	}
	return nil
}
func accountWriteError(err error) error {
	var mysqlErr *mysqldriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return ErrNameAlreadyExists
	}
	return err
}
func (s *AccountService) CreateAccount(ctx context.Context, a *Account) error {
	if a == nil {
		return apierror.ErrValidation
	}
	a.Username = strings.TrimSpace(a.Username)
	if err := validateUsername(a.Username); err != nil {
		return err
	}
	if err := validatePassword(a.Password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(a.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	a.Password = string(hash)
	return accountWriteError(s.repo.CreateAccount(ctx, a))
}
func (s *AccountService) Login(ctx context.Context, name, password string) (string, string, error) {
	a, err := s.repo.FindOneUserByUserame(ctx, strings.TrimSpace(name))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", ErrInvalidUsername
	}
	if err != nil {
		return "", "", err
	}
	if err := VerifyPassword(ctx, a.Password, password); err != nil {
		return "", "", ErrInvalidPassword
	}
	access := auth.GenerateToken(a.ID, a.Username)
	refresh, err := auth.GenerateRefreshToken()
	if err != nil {
		return "", "", err
	}
	if err := s.repo.Login(ctx, a.ID, a.Password, access, refresh, time.Now().UTC().Add(refreshLifetime)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", ErrInvalidPassword
		}
		return "", "", err
	}
	s.clearLegacySessionCache(a)
	return access, refresh, nil
}

// Authenticate deliberately reads the authoritative session from MySQL.
// Redis outages or stale cache entries cannot make a revoked token valid.
func (s *AccountService) Authenticate(ctx context.Context, token string) (*Account, error) {
	claims, err := auth.ParseToken(token)
	if err != nil {
		return nil, ErrInvalidToken
	}
	a, err := s.repo.FindUserByAccountID(ctx, claims.AccountID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	if a.Token == "" || a.Token != token {
		return nil, ErrInvalidToken
	}
	return a, nil
}
func (s *AccountService) Logout(ctx context.Context, id uint) error {
	a, err := s.repo.FindUserByAccountID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.Logout(ctx, id); err != nil {
		return err
	}
	s.clearLegacySessionCache(a)
	return nil
}
func (s *AccountService) ChangePassword(ctx context.Context, id uint, oldPassword, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	a, err := s.repo.FindUserByAccountID(ctx, id)
	if err != nil {
		return err
	}
	if err := VerifyPassword(ctx, a.Password, oldPassword); err != nil {
		return ErrInvalidPassword
	}
	if VerifyPassword(ctx, a.Password, newPassword) == nil {
		return fmt.Errorf("new password must differ from old password: %w", apierror.ErrValidation)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.repo.ChangePassword(ctx, id, a.Password, string(hash)); err != nil {
		return err
	}
	s.clearLegacySessionCache(a)
	return nil
}
func (s *AccountService) Rename(ctx context.Context, id uint, name string) (string, error) {
	name = strings.TrimSpace(name)
	if err := validateUsername(name); err != nil {
		return "", err
	}
	a, err := s.repo.FindUserByAccountID(ctx, id)
	if err != nil {
		return "", err
	}
	if a.Token == "" {
		return "", ErrInvalidToken
	}
	token := auth.GenerateToken(id, name)
	if err := s.repo.RenameWithToken(ctx, a.Token, token, id, name); err != nil {
		return "", accountWriteError(err)
	}
	s.clearLegacySessionCache(a)
	return token, nil
}
func (s *AccountService) RefreshAccessToken(ctx context.Context, refresh string) (string, uint, string, error) {
	if len(refresh) != 64 {
		return "", 0, "", ErrInvalidToken
	}
	now := time.Now().UTC()
	a, err := s.repo.FindByRefreshToken(ctx, refresh, now)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", 0, "", ErrInvalidToken
	}
	if err != nil {
		return "", 0, "", err
	}
	token := auth.GenerateToken(a.ID, a.Username)
	if err := s.repo.RefreshAccessToken(ctx, a.ID, refresh, token, time.Now().UTC()); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", 0, "", ErrInvalidToken
		}
		return "", 0, "", err
	}
	s.clearLegacySessionCache(a)
	return token, a.ID, a.Username, nil
}
func (s *AccountService) clearLegacySessionCache(a *Account) {
	if s.cache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	keys := []string{s.cache.Key("account:%d", a.ID), s.cache.Key("accountID:%d", a.ID), s.cache.Key("account:%d:refresh", a.ID)}
	if a.RefreshToken != "" {
		keys = append(keys, s.cache.Key("refresh:%s", a.RefreshToken))
	}
	for _, key := range keys {
		if err := s.cache.Del(ctx, key); err != nil {
			log.Printf("legacy session cache cleanup failed: %v", err)
			return
		}
	}
}
func (s *AccountService) FindByID(ctx context.Context, id uint) (*Account, error) {
	return s.repo.FindUserByAccountID(ctx, id)
}
func (s *AccountService) FindByUsername(ctx context.Context, name string) (*Account, error) {
	return s.repo.FindOneUserByUserame(ctx, name)
}
func (s *AccountService) FindAll(ctx context.Context) ([]*Account, error) {
	return s.repo.FindAllAccounts(ctx)
}
func (s *AccountService) UpdateAvatar(ctx context.Context, id uint, url string) error {
	return s.repo.UpdateAvatar(ctx, id, url)
}
func (s *AccountService) UpdateProfile(ctx context.Context, id uint, req *UpdateProfileRequest) error {
	updates := map[string]interface{}{}
	if req.Bio != "" {
		updates["bio"] = strings.TrimSpace(req.Bio)
	}
	if req.AvatarURL != "" {
		updates["avatar_url"] = strings.TrimSpace(req.AvatarURL)
	}
	if len(updates) == 0 {
		return apierror.ErrValidation
	}
	return s.repo.UpdateFields(ctx, id, updates)
}
