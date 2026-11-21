package system

import (
	"sun-panel/api/api_v1"

	"github.com/gin-gonic/gin"
)

func InitCaptchaRouter(router *gin.RouterGroup) {
	api := api_v1.ApiGroupApp.ApiSystem.CaptchaApi
	router.GET("/captcha/getImage", api.GetImage)
	router.GET("/captcha/getImage/:width/:height", api.GetImage)
}
