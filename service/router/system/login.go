package system

import (
	"sun-panel/api/api_v1"
	"sun-panel/api/api_v1/middleware"

	"github.com/gin-gonic/gin"
)

func InitLogin(router *gin.RouterGroup) {
	loginApi := api_v1.ApiGroupApp.ApiSystem.LoginApi

	router.POST("/login", loginApi.Login)
	// Passkey 登录为 public 接口：不依赖已登录会话（挑战与当前会话绑定，未登录时绑定值为空）
	router.POST("/login/passkey/begin", loginApi.PasskeyLoginBegin)
	router.POST("/login/passkey/finish", loginApi.PasskeyLoginFinish)
	router.POST("/logout", middleware.LoginInterceptor, loginApi.Logout)

}
