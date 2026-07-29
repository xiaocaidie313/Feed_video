package account

import (
	"context"
	"errors"
	"feedvid/internal/auth"
	"log"

	"golang.org/x/crypto/bcrypt"
)

type AccountService struct {
	repo *AccountRepo
}

var (
	ErrInvalidName       = errors.New("name is invalid")
	ErrInvalidToken      = errors.New("token is invalid")
	ErrNameAlreadyExists = errors.New("name already exists")
	ErrNameIsEmpty       = errors.New("name is required")
)

// 需要 完成 业务逻辑: 初始化一个account并写好  把写好的account 传入数据库
func (as *AccountService) CreateAccount(ctx context.Context, acc *Account) error {
	// 加密密码  account里面存的是明文密码 需要加密后存入数据库
	hashedPassWord, err := bcrypt.GenerateFromPassword([]byte(acc.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("failed to generate password: %v", err)
		return err
	}
	acc.Password = string(hashedPassWord)
	if err := as.repo.CreateAccount(ctx, acc); err != nil {
		return err
	}
	return nil
}

func (as *AccountService) RenameWithToken(ctx context.Context, id uint, newName string) (string, error) {
	//@todo  需要要求 新旧名字不一样吗
	if newName == "" {
		return " ", ErrNameIsEmpty
	}
	// 重命名 重新生成一个新的token
	token := auth.GenerateToken(id, newName)
	if err := as.repo.RenameWithToken(ctx, token, id, newName); err != nil {
		return "", ErrInvalidToken
	}
	return token, nil
}

func (as *AccountService) Login(){}
func (as *AccountService) Logout(){}
func (as *AccountService) ChangePassword(){}
func (as *AccountService) EditProfile(){}

