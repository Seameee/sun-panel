package system

import (
	"sun-panel/api/api_v1"
	"sun-panel/api/api_v1/middleware"

	"github.com/gin-gonic/gin"
)

// InitSecurityRouter 二次认证管理路由（TOTP / Passkey）
//
// 全部挂在 LoginInterceptor 之后：必须已登录才能管理自己的二次认证。
func InitSecurityRouter(router *gin.RouterGroup) {
	api := api_v1.ApiGroupApp.ApiSystem.SecurityApi
	r := router.Group("", middleware.LoginInterceptor)
	{
		r.POST("/user/security/status", api.Status)
		r.POST("/user/security/totp/begin", api.TotpBegin)
		r.POST("/user/security/totp/finish", api.TotpFinish)
		r.POST("/user/security/totp/disable", api.TotpDisable)
		r.POST("/user/security/passkey/begin", api.PasskeyBegin)
		r.POST("/user/security/passkey/finish", api.PasskeyFinish)
		r.POST("/user/security/passkey/delete", api.PasskeyDelete)
	}
}
