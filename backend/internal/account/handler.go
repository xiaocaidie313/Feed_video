package account

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"feedvid/internal/apierror"
	"feedvid/internal/auth"
	"log"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AccountHandler struct {
	accountService *AccountService
}

func NewAccountHandler(accountService *AccountService) *AccountHandler {
	return &AccountHandler{accountService: accountService}
}
func (h *AccountHandler) CreateAccount(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "username and password are required"})
		return
	}
	if err := h.accountService.CreateAccount(c.Request.Context(), &Account{Username: req.Username, Password: req.Password}); err != nil {
		writeAccountError(c, err)
		return
	}
	c.JSON(200, gin.H{"message": "account created"})
}

func (h *AccountHandler) Rename(c *gin.Context) {
	var req RenameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	id, err := getAccountID(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "missing account"})
		return
	}
	token, err := h.accountService.Rename(c.Request.Context(), id, req.NewUsername)
	if err != nil {
		writeAccountError(c, err)
		return
	}
	c.JSON(200, gin.H{"token": token})
}

func (h *AccountHandler) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "old_password and new_password are required"})
		return
	}
	id, err := getAccountID(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "missing account"})
		return
	}
	if err := h.accountService.ChangePassword(c.Request.Context(), id, req.OldPassword, req.NewPassword); err != nil {
		writeAccountError(c, err)
		return
	}
	c.JSON(200, gin.H{"message": "password changed; please log in again"})
}

func (h *AccountHandler) FindByID(c *gin.Context) {
	var req FindByIDRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(apierror.MathErrorsToStatus(err), gin.H{"error": err.Error()})
		return
	}
	if account, err := h.accountService.FindByID(c.Request.Context(), req.ID); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	} else {
		c.JSON(200, account)
	}
}

func (h *AccountHandler) FindByUsername(c *gin.Context) {
	var req FindByUsernameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(apierror.MathErrorsToStatus(err), gin.H{"error": err.Error()})
		return
	}
	if account, err := h.accountService.FindByUsername(c.Request.Context(), req.Username); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	} else {
		c.JSON(200, account)
	}
}

func (h *AccountHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" {
		c.JSON(400, gin.H{"error": "username and password are required"})
		return
	}
	access, refresh, err := h.accountService.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		writeAccountError(c, err)
		return
	}
	claims, err := auth.ParseToken(access)
	if err != nil {
		writeAccountError(c, err)
		return
	}
	c.JSON(200, LoginResponse{Token: access, RefreshToken: refresh, AccountID: claims.AccountID, Username: claims.UserName})
}

func (h *AccountHandler) Logout(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		c.JSON(apierror.MathErrorsToStatus(err), gin.H{"error": err.Error()})
		return
	}
	if err := h.accountService.Logout(c.Request.Context(), accountID); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "account logged out"})
}

func (h *AccountHandler) UploadAvatar(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	f, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing file"})
		return
	}
	const maxSize = 10 << 20
	if f.Size <= 0 || f.Size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file size"})
		return
	}
	ext := strings.ToLower(filepath.Ext(f.Filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "only .jpg/.jpeg/.png/.webp allowed"})
		return
	}
	dir := filepath.Join(".run", "uploads", "avatars", strconv.FormatUint(uint64(accountID), 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filename, err := randHex(16)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filename = filename + ext
	absPath := filepath.Join(dir, filename)
	if err := c.SaveUploadedFile(f, absPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	urlPath := path.Join("/static", "avatars", strconv.FormatUint(uint64(accountID), 10), filename)
	avatarURL := buildAbsoluteURL(c, urlPath)
	if err := h.accountService.UpdateAvatar(c.Request.Context(), accountID, avatarURL); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"avatar_url": avatarURL})
}

func (h *AccountHandler) UpdateProfile(c *gin.Context) {
	accountID, err := getAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(apierror.MathErrorsToStatus(err), gin.H{"error": err.Error()})
		return
	}
	if err := h.accountService.UpdateProfile(c.Request.Context(), accountID, &req); err != nil {
		c.JSON(apierror.MathErrorsToStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "profile updated"})
}

func (h *AccountHandler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
		c.JSON(400, gin.H{"error": "refresh_token is required"})
		return
	}
	token, id, name, err := h.accountService.RefreshAccessToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		writeAccountError(c, err)
		return
	}
	c.JSON(200, LoginResponse{Token: token, RefreshToken: req.RefreshToken, AccountID: id, Username: name})
}

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand.Read: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func buildAbsoluteURL(c *gin.Context, p string) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if xf := c.GetHeader("X-Forwarded-Proto"); xf != "" {
		scheme = xf
	}
	return fmt.Sprintf("%s://%s%s", scheme, c.Request.Host, p)
}

func getAccountID(c *gin.Context) (uint, error) {
	value, exists := c.Get("accountID")
	if !exists {
		return 0, errors.New("accountID not found")
	}
	id, ok := value.(uint)
	if !ok {
		return 0, errors.New("accountID has invalid type")
	}
	return id, nil
}

func writeAccountError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidUsername), errors.Is(err, ErrInvalidPassword), errors.Is(err, ErrInvalidToken), errors.Is(err, apierror.ErrUnauthorized):
		c.JSON(401, gin.H{"error": "invalid credentials or revoked session"})
	case errors.Is(err, apierror.ErrValidation), errors.Is(err, ErrInvalidName), errors.Is(err, ErrNameIsEmpty):
		c.JSON(400, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNameAlreadyExists):
		c.JSON(409, gin.H{"error": "username already exists"})
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(404, gin.H{"error": "account not found"})
	default:
		log.Printf("account operation failed: %v", err)
		c.JSON(500, gin.H{"error": "internal server error"})
	}
}
