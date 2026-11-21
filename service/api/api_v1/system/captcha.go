package system

import (
	"encoding/base64"
	"net/http"
	"strings"
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/global"
	"sun-panel/lib/captcha"
	"sun-panel/lib/cmn/systemSetting"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CaptchaApi struct{}

func captchaEnabled() bool {
	var settings systemSetting.ApplicationSetting
	if global.SystemSetting != nil {
		_ = global.SystemSetting.GetValueByInterface("system_application", &settings)
	}
	return settings.LoginCaptcha
}

func (LoginApi) Options(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	apiReturn.SuccessData(c, gin.H{"captchaEnabled": captchaEnabled()})
}

func (CaptchaApi) GetImage(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !captchaEnabled() {
		c.Status(http.StatusNotFound)
		return
	}
	id := uuid.NewString()
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("CaptchaId", id, 300, "/api", "", c.Request.TLS != nil, true)
	_, contents, ok := strings.Cut(captcha.GenerateCaptchaHandler(id, 120, 40), ",")
	if !ok {
		c.Status(http.StatusInternalServerError)
		return
	}
	image, err := base64.StdEncoding.DecodeString(contents)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(http.StatusOK, "image/png", image)
}
