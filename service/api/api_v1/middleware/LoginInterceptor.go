package middleware

import (
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/global"
	"sun-panel/lib/security"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
)

func sessionUser(c *gin.Context) (models.User, bool) {
	client := c.GetHeader("token")
	if client == "" || len(client) > 128 {
		return models.User{}, false
	}
	generation, ok := global.CUserToken.Get(client)
	if !ok || generation == "" {
		return models.User{}, false
	}
	// The database is authoritative so resets from a separate CLI process take effect immediately.
	var model models.User
	info, err := model.GetUserInfoByToken(generation)
	return info, err == nil && info.ID != 0 && info.Status == 1
}

func mustChangePassword(c *gin.Context, user models.User) bool {
	if !security.RequiresPasswordChange(user.Password) {
		return false
	}
	apiReturn.ErrorByCode(c, 1105)
	c.Abort()
	return true
}

func LoginInterceptor(c *gin.Context) {
	if c.GetHeader("token") == "" {
		apiReturn.ErrorByCode(c, 1000)
		c.Abort()
		return
	}
	info, ok := sessionUser(c)
	if !ok {
		apiReturn.ErrorByCode(c, 1001)
		c.Abort()
		return
	}
	switch c.FullPath() {
	case "/api/user/updatePassword", "/api/logout":
	default:
		if mustChangePassword(c, info) {
			return
		}
	}
	c.Set("userInfo", info)
}

// 不验证缓存直接验证库省去没有缓存每次都要手动登录的问题
func LoginInterceptorDev(c *gin.Context) {
	LoginInterceptor(c)
}
