package social

import (
	"net/http"

	"feedvid/internal/midware/jwt"

	"feedvid/internal/account"
	"feedvid/internal/apierror"

	"github.com/gin-gonic/gin"
)

type SocialHandler struct {
	service *SocialService
}

func NewSocialHandler(service *SocialService) *SocialHandler {
	return &SocialHandler{
		service: service,
	}
}

func (sh *SocialHandler) Follow(c *gin.Context) {
	var req FollowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, err := sh.service.accountRepo.FindUserByAccountID(c.Request.Context(), req.VloggerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	followerID, err := jwt.GetAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	err = sh.service.Follow(c.Request.Context(), &Social{
		FollowerID: followerID,
		VloggerID:  req.VloggerID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Followed successfully"})

}

func (sh *SocialHandler) UnFollow(c *gin.Context) {
	var req FollowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, err := sh.service.accountRepo.FindUserByAccountID(c.Request.Context(), req.VloggerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	followerID, err := jwt.GetAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	err = sh.service.UnFollow(c.Request.Context(), &Social{
		FollowerID: followerID,
		VloggerID:  req.VloggerID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "UnFollowed successfully"})
}

func (sh *SocialHandler) CountAllFollowers(c *gin.Context) {
	var req GetAllFollowersRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(apierror.MathErrorsToStatus(err), gin.H{"error": err.Error()})
		return
	}

	followers, err := sh.service.GetAllFollowers(c.Request.Context(), req.VloggerID)
	if err != nil {
		c.JSON(apierror.MathErrorsToStatus(err), gin.H{"error": err.Error()})
		return
	}
	if followers == nil {
		followers = []*account.Account{}
	}
	followerCount, _ := sh.service.CountAllFollowers(c.Request.Context(), req.VloggerID)
	c.JSON(http.StatusOK, GetAllFollowersResponse{Followers: followers, FollowerCount: followerCount})
}

func (sh *SocialHandler) CountAllVloggers(c *gin.Context) {
	var req GetAllVloggersRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(apierror.MathErrorsToStatus(err), gin.H{"error": err.Error()})
		return
	}

	vloggers, err := sh.service.GetAllVollgers(c.Request.Context(), req.FollowerID)
	if err != nil {
		c.JSON(apierror.MathErrorsToStatus(err), gin.H{"error": err.Error()})
		return
	}
	if vloggers == nil {
		vloggers = []*account.Account{}
	}
	vloggerCount, _ := sh.service.CountAllVollgers(c.Request.Context(), req.FollowerID)
	c.JSON(http.StatusOK, GetAllVloggersResponse{Vloggers: vloggers, VloggerCount: vloggerCount})
}

func (sh *SocialHandler) GetAllVloggers(c *gin.Context) {
	var req GetAllVloggersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(apierror.MathErrorsToStatus(err), gin.H{"error": err.Error()})
		return
	}

	followerID := req.FollowerID
	if followerID == 0 {
		accountID, err := jwt.GetAccountID(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		followerID = accountID
	}

	vloggers, err := sh.service.GetAllVollgers(c.Request.Context(), followerID)
	if err != nil {
		c.JSON(apierror.MathErrorsToStatus(err), gin.H{"error": err.Error()})
		return
	}
	if vloggers == nil {
		vloggers = []*account.Account{}
	}
	vloggerCount, _ := sh.service.CountAllVollgers(c.Request.Context(), followerID)
	c.JSON(http.StatusOK, GetAllVloggersResponse{Vloggers: vloggers, VloggerCount: vloggerCount})
}

func (h *SocialHandler) GetAllFollowers(c *gin.Context) {
	var req GetAllFollowersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(apierror.ClassifyHTTPStatus(err), gin.H{"error": err.Error()})
		return
	}

	vloggerID := req.VloggerID
	if vloggerID == 0 {
		accountID, err := jwt.GetAccountID(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		vloggerID = accountID
	}

	followers, err := h.service.GetAllFollowers(c.Request.Context(), vloggerID)
	if err != nil {
		c.JSON(apierror.ClassifyHTTPStatus(err), gin.H{"error": err.Error()})
		return
	}
	if followers == nil {
		followers = []*account.Account{}
	}
	followerCount, _ := h.service.CountFollowers(c.Request.Context(), vloggerID)
	c.JSON(http.StatusOK, GetAllFollowersResponse{Followers: followers, FollowerCount: followerCount})
}

func (sh *SocialHandler) GetCount(c *gin.Context) {
	accountID, err := jwt.GetAccountID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	followerCount, err := sh.service.CountAllFollowers(c.Request.Context(), accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	vloggerCount, err := sh.service.CountAllVollgers(c.Request.Context(), accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, SocialCounts{FollowerCount: followerCount, VloggerCount: vloggerCount})
}
