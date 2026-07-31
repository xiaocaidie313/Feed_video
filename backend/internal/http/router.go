package http

import (
	"feedvid/internal/account"
	"feedvid/internal/midware/jwt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetRouter(db *gorm.DB) {
	r := gin.Default()

	// 健康检查
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})

	r.Static("/static", "./.run/uploads")
	// @Todo   redis 限流器
	// account group
	accountRepository := account.NewAccountRepo(db)
	accountService := account.NewAccountService(accountRepository)
	accountHandler := account.NewAccountHandler(accountService)

	accountGroup := r.Group("/api/account")
	{
		accountGroup.POST("/register", accountHandler.CreateAccount) // 将func写入路由表
		accountGroup.POST("/login", accountHandler.Login)
		accountGroup.POST("/change-password", accountHandler.ChangePassword)
		accountGroup.POST("/findbyid", accountHandler.FindByID)
		accountGroup.POST("/findbyusername", accountHandler.FindByUsername)
		// accountGroup.POST("/update-profile", accountHandler.UpdateProfile)
		accountGroup.POST("/refresh", accountHandler.Refresh)
	}
	protectedAccountGroup := accountGroup.Group("")
	protectedAccountGroup.Use(jwt.JWTAuth(accountService))
	{
		protectedAccountGroup.POST("/logout", accountHandler.Logout)
		protectedAccountGroup.POST("/rename", accountHandler.Rename)
		protectedAccountGroup.POST("/uploadAvatar", accountHandler.UploadAvatar)
		protectedAccountGroup.POST("/updateProfile", accountHandler.UpdateProfile)
	}

}
