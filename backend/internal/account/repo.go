package account

import (
	"context"
	"gorm.io/gorm"
	"time"
)

type AccessToken string
type RefreshToken string
type AccountRepo struct{ db *gorm.DB }

func NewAccountRepo(db *gorm.DB) *AccountRepo { return &AccountRepo{db: db} }
func (r *AccountRepo) CreateAccount(ctx context.Context, a *Account) error {
	return r.db.WithContext(ctx).Create(a).Error
}
func (r *AccountRepo) FindOneUserByUserame(ctx context.Context, name string) (*Account, error) {
	var a Account
	err := r.db.WithContext(ctx).Where("username = ?", name).First(&a).Error
	return &a, err
}
func (r *AccountRepo) FindUserByAccountID(ctx context.Context, id uint) (*Account, error) {
	var a Account
	err := r.db.WithContext(ctx).First(&a, id).Error
	return &a, err
}
func (r *AccountRepo) Find20UsersByUsername(ctx context.Context, name string) ([]Account, error) {
	var accounts []Account
	err := r.db.WithContext(ctx).Where("username LIKE ?", "%"+name+"%").Limit(20).Find(&accounts).Error
	return accounts, err
}
func requireChanged(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
func (r *AccountRepo) Rename(ctx context.Context, id uint, name string) error {
	return requireChanged(r.db.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Update("username", name))
}
func (r *AccountRepo) RenameWithToken(ctx context.Context, oldToken, newToken string, id uint, name string) error {
	return requireChanged(r.db.WithContext(ctx).Model(&Account{}).Where("id = ? AND token = ?", id, oldToken).
		Updates(map[string]interface{}{"username": name, "token": newToken}))
}
func (r *AccountRepo) Login(ctx context.Context, id uint, passwordHash, access, refresh string, expires time.Time) error {
	// The password predicate prevents an in-flight old-password login from
	// restoring a session after a concurrent password change.
	return requireChanged(r.db.WithContext(ctx).Model(&Account{}).Where("id = ? AND password = ?", id, passwordHash).
		Updates(map[string]interface{}{"token": access, "refresh_token": refresh, "refresh_expires_at": expires}))
}
func (r *AccountRepo) Logout(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&Account{}).Where("id = ?", id).
		Updates(map[string]interface{}{"token": "", "refresh_token": "", "refresh_expires_at": nil}).Error
}
func (r *AccountRepo) ChangePassword(ctx context.Context, id uint, oldHash, newHash string) error {
	// A single UPDATE changes the password and revokes both tokens atomically.
	return requireChanged(r.db.WithContext(ctx).Model(&Account{}).Where("id = ? AND password = ?", id, oldHash).
		Updates(map[string]interface{}{"password": newHash, "token": "", "refresh_token": "", "refresh_expires_at": nil}))
}
func (r *AccountRepo) FindByRefreshToken(ctx context.Context, token string, now time.Time) (*Account, error) {
	var a Account
	err := r.db.WithContext(ctx).Where("refresh_token = ? AND refresh_expires_at > ?", token, now).First(&a).Error
	return &a, err
}
func (r *AccountRepo) RefreshAccessToken(ctx context.Context, id uint, refresh, access string, now time.Time) error {
	// Recheck the refresh token in the UPDATE so logout cannot be undone by a
	// refresh request that read the old session just before logout.
	return requireChanged(r.db.WithContext(ctx).Model(&Account{}).
		Where("id = ? AND refresh_token = ? AND refresh_expires_at > ?", id, refresh, now).
		Update("token", access))
}
func (r *AccountRepo) UpdateAvatar(ctx context.Context, id uint, url string) error {
	return r.UpdateFields(ctx, id, map[string]interface{}{"avatar_url": url})
}
func (r *AccountRepo) UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Updates(updates).Error
}
func (r *AccountRepo) FindAllAccounts(ctx context.Context) ([]*Account, error) {
	var accounts []*Account
	err := r.db.WithContext(ctx).Find(&accounts).Error
	return accounts, err
}
