package account

import (
	"context"
	"log"

	"gorm.io/gorm"
)

type AccessToken string
type RefreshToken string

type AccountRepo struct {
	db *gorm.DB
}

// 单纯的把 account 写入数据库  是初始化好的accout
func (ar *AccountRepo) CreateAccount(ctx context.Context, acc *Account) error {

	if err := ar.db.WithContext(ctx).Create(acc).Error; err != nil {
		log.Printf("failed to create account: %v", err)
		return err
	}
	return nil
}

func (ar *AccountRepo) Rename(ctx context.Context, id uint, newUsername string) error {
	result := ar.db.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Update("username", newUsername)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// 重命名 并更新token
func (ar *AccountRepo) RenameWithToken(ctx context.Context, token string, id uint, newUsername string) error {
	//@todo 学习一下gorm的transaction
	return ar.db.Transaction(func(tx *gorm.DB) error {
		result := tx.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Update("username", newUsername)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Update("token", token).Error; err != nil {
			return err
		}
		return nil
	})

}

// 查找单个用户
func (ar *AccountRepo) FindOneUserByUserame(ctx context.Context, username string) (*Account, error) {
	var acc Account
	if err := ar.db.WithContext(ctx).Where("username = ?", username).First(&acc).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

// 查找单个用户 by  id
func (ar *AccountRepo) FindUserByAccountID(ctx context.Context, id uint) (*Account, error) {
	var acc Account
	if err := ar.db.WithContext(ctx).First(&acc, id).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

// 找20个同名用户  // @todo 判断最大时候超过 20
func (ar *AccountRepo) Find20UsersByUsername(ctx context.Context, username string) ([]Account, error) {
	var accs []Account
	if err := ar.db.WithContext(ctx).
		Where("username LIKE ?", "%"+username+"%").
		Limit(20).
		Find(&accs).Error; err != nil {
		return nil, err
	}
	return accs, nil
}

func (ar *AccountRepo) Login(ctx context.Context, id uint, accessToken string, refreshToken string) error {
	ar.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Update("access_token", accessToken).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Update("refresh_token", refreshToken).Error; err != nil {
			return err
		}
		return nil
	})
	return nil
}

// 登出清除
func (ar *AccountRepo) Logout(ctx context.Context, id uint) error {
	if err := ar.db.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Update("access_token", "").Error; err != nil {
		return err
	}
	if err := ar.db.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Update("refresh_token", "").Error; err != nil {
		return err
	}
	return nil
}

func (ar *AccountRepo) ChangePassword(ctx context.Context, id uint, newHashedPassword string) error {
	return ar.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Update("password", newHashedPassword).Error; err != nil {
			return err
		}
		return nil
	})
}

func (ar *AccountRepo) UpdateAvatar(ctx context.Context, accountID uint, avatarURL string) error {
	return ar.db.WithContext(ctx).Model(&Account{}).Where("id = ?", accountID).Update("avatar_url", avatarURL).Error
}

func (ar *AccountRepo) UpdateToken(ctx context.Context, id uint, token string) error {
	return ar.db.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Update("token", token).Error
}

func (ar *AccountRepo) UpdateFields(ctx context.Context, id uint, updates map[string]interface{}) error {
	return ar.db.WithContext(ctx).Model(&Account{}).Where("id = ?", id).Updates(updates).Error
}

func (ar *AccountRepo) FindAllAccounts(ctx context.Context) ([]*Account, error) {
	var accounts []*Account
	if err := ar.db.WithContext(ctx).Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}
