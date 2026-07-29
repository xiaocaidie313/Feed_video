package account

import (
	"context"
	"log"

	"gorm.io/gorm"
)

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
