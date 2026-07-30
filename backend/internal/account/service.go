package account

import (
	"context"
	"errors"
	"feedvid/internal/auth"
	"log"
	"strings"
	"strconv"
	"time"
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
	ErrInvalidUsername   = errors.New("username is invalid")
	ErrInvalidPassword   = errors.New("password is invalid")
)

// 验证密码
func VerifyPassword(ctx context.Context, hashedPassword string, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)); err != nil {
		log.Printf("failed to verify password: %v", err)
		return err
	}
	return nil
}

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

func (as *AccountService) Login(ctx context.Context, username string, password string) (string, string, error) {
	acc, err := as.repo.FindOneUserByUserame(ctx, username)
	if err != nil {
		return "", "", ErrInvalidUsername
	}

	if err := VerifyPassword(ctx, password, acc.Password); err != nil {
		return "", "", ErrInvalidPassword
	}

	accessToken := auth.GenerateToken(acc.ID, acc.Username)
	if err != nil {
		return "", "", err
	}
	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		return "", "", err
	}

	if err := as.repo.Login(ctx, acc.ID, accessToken, refreshToken); err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil

}
func (as *AccountService) Logout(ctx context.Context, accountID uint) error {
	// acc, err := as.repo.FindUserByAccountID(ctx, accountID)
	// if err != nil {
	// 	return err
	// }
	// if acc.Token == "" {
	// 	return nil
	// }
	// if err := as.repo.Logout(ctx, acc.ID); err != nil {
	// 	return err
	// }
	return as.repo.Logout(ctx, accountID)

}
func (as *AccountService) ChangePassword(ctx context.Context, username string, newpassword string) error {
	acc, _ := as.repo.FindOneUserByUserame(ctx, username)

	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(newpassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := VerifyPassword(ctx, string(newHashedPassword), acc.Password); err != nil {
		log.Printf("failed to verify password: %v", err)
		// 新旧密码一样报错
		return ErrInvalidPassword
	}

	return as.repo.ChangePassword(ctx, acc.ID, string(newHashedPassword))
}

func (as *AccountService) EditProfile(ctx context.Context, username string, newName string) error {
	acc, _ := as.repo.FindOneUserByUserame(ctx, username)
	if newName == "" {
		return ErrNameIsEmpty
	}
	return nil
}
func (as *AccountService) FindByID(ctx context.Context, id uint) (*Account, error) {
	if account, err := as.repo.FindUserByAccountID(ctx, id); err != nil {
		return nil, err
	} else {
		return account, nil
	}
}

func (as *AccountService) FindByUsername(ctx context.Context, username string) (*Account, error) {
	if account, err := as.repo.FindOneUserByUserame(ctx, username); err != nil {
		return nil, err
	} else {
		return account, nil
	}
}
func (as *AccountService) UpdateAvatar(ctx context.Context, accountID uint, avatarURL string) error {
	return as.repo.UpdateAvatar(ctx, accountID, avatarURL)
}

func (as *AccountService) FindAll(ctx context.Context) ([]*Account, error) {
	return as.repo.FindAllAccounts(ctx)
}



func (as *AccountService) UpdateProfile(ctx context.Context, accountID uint, req *UpdateProfileRequest) error {
	updates := map[string]interface{}{}
	if req.Bio != "" {
		updates["bio"] = strings.TrimSpace(req.Bio)
	}
	if req.AvatarURL != "" {
		updates["avatar_url"] = strings.TrimSpace(req.AvatarURL)
	}
	if len(updates) == 0 {
		return errors.New("nothing to update")
	}
	return as.repo.UpdateFields(ctx, accountID, updates)
}

func (as *AccountService) RefreshAccessToken(ctx context.Context, refreshToken string) (string, uint, string, error) {
	if refreshToken == "" {
		return "", 0, "", errors.New("refresh token is empty")
	}
	if as.cache != nil {
		cacheCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		b, err := as.cache.GetBytes(cacheCtx, as.cache.Key("refresh:%s", refreshToken))
		if err == nil {
			idStr := string(b)
			id, parseErr := strconv.ParseUint(idStr, 10, 64)
			if parseErr == nil {
				account, err := as.FindByID(ctx, uint(id))
				if err == nil && account != nil && account.RefreshToken == refreshToken {
					newToken, err := auth.GenerateToken(account.ID, account.Username)
					if err != nil {
						return "", 0, "", err
					}
					if err := as.accountRepository.UpdateToken(ctx, account.ID, newToken); err != nil {
						return "", 0, "", err
					}
					if err := as.cache.SetBytes(cacheCtx, as.cache.Key("account:%d", account.ID), []byte(newToken), 24*time.Hour); err != nil {
						log.Printf("failed to set cache: %v", err)
					}
					return newToken, account.ID, account.Username, nil
				}
			}
		}
	}
	accounts, err := as.FindAll(ctx)
	if err != nil {
		return "", 0, "", err
	}
	for _, acc := range accounts {
		if acc.RefreshToken == refreshToken {
			newToken, err := auth.GenerateToken(acc.ID, acc.Username)
			if err != nil {
				return "", 0, "", err
			}
			if err := as.accountRepository.UpdateToken(ctx, acc.ID, newToken); err != nil {
				return "", 0, "", err
			}
			return newToken, acc.ID, acc.Username, nil
		}
	}
	return "", 0, "", errors.New("invalid refresh token")
}

