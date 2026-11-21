package system

import (
	"errors"
	"strings"
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/biz"
	"sun-panel/global"
	"sun-panel/lib/captcha"
	"sun-panel/lib/security"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type LoginApi struct {
}

// 登录输入验证
type LoginLoginVerify struct {
	Username string `json:"username" validate:"required,max=254"`
	Password string `json:"password" validate:"required,max=200"`
	VCode    string `json:"vcode" validate:"max=6"`
	Email    string `json:"email"`
	// TotpCode 为可选的二次验证码；启用 TOTP 的账号不提交时会返回 1101
	TotpCode string `json:"totpCode" validate:"max=16"`
}

// @Summary 登录账号
// @Accept application/json
// @Produce application/json
// @Param LoginLoginVerify body LoginLoginVerify true "登陆验证信息"
// @Tags user
// @Router /login [post]
func (l LoginApi) Login(c *gin.Context) {
	param := LoginLoginVerify{}
	if err := c.ShouldBindJSON(&param); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	if errMsg, err := base.ValidateInputStruct(param); err != nil {
		apiReturn.ErrorParamFomat(c, errMsg)
		return
	}

	param.Username = strings.TrimSpace(param.Username)
	if !allowPasswordAttempt(c, param.Username) {
		return
	}

	if captchaEnabled() {
		id, _ := c.Cookie("CaptchaId")
		if id == "" || !captcha.CaptchaVerifyHandle(id, param.VCode) {
			apiReturn.ErrorByCode(c, 1103)
			return
		}
	}

	var info models.User
	err := global.Db.Where("username = ?", param.Username).First(&info).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiReturn.ErrorByCode(c, 1003)
		return
	}
	if err != nil {
		apiReturn.ErrorDatabase(c, "")
		return
	}
	valid, legacy := security.VerifyPassword(param.Password, info.Password)
	if !valid {
		apiReturn.ErrorByCode(c, 1003)
		return
	}

	// 停用或未激活
	if info.Status != 1 {
		apiReturn.ErrorByCode(c, 1004)
		return
	}

	// 二次验证（TOTP）：必须在签发 token 之前完成，未启用 TOTP 的账号行为不变
	if err := biz.Security.VerifyLoginTotp(info.ID, param.TotpCode); err != nil {
		switch {
		case errors.Is(err, biz.ErrSecurityTotpRequired):
			apiReturn.ErrorByCode(c, apiReturn.ERROR_CODE_VERIFICATION_MUST)
		case errors.Is(err, biz.ErrSecurityTotpInvalid), errors.Is(err, biz.ErrSecurityTotpLocked):
			apiReturn.ErrorByCode(c, apiReturn.ERROR_CODE_VERIFICATION_FAIL)
		default:
			global.Logger.Errorln("Login totp error:", err.Error())
			apiReturn.ErrorDatabase(c, err.Error())
		}
		return
	}

	if legacy && !security.RequiresPasswordChange(info.Password) {
		upgraded, err := security.HashPassword(param.Password)
		if err != nil {
			apiReturn.ErrorDatabase(c, "")
			return
		}
		result := global.Db.Model(&models.User{}).Where("id = ? AND password = ?", info.ID, info.Password).UpdateColumn("password", upgraded)
		if result.Error != nil {
			apiReturn.ErrorDatabase(c, "")
			return
		}
		if result.RowsAffected != 1 {
			apiReturn.ErrorByCode(c, 1003)
			return
		}
		info.Password = upgraded
	}
	l.issueSession(c, info)
}

func allowPasswordAttempt(c *gin.Context, account string) bool {
	if security.PasswordAttempts.Allow(account, security.PasswordSource(c), time.Now()) {
		return true
	}
	c.Header("Retry-After", "300")
	apiReturn.ErrorByCode(c, 1104)
	return false
}

// issueSession binds an opaque client token to the current database session generation.
func (LoginApi) issueSession(c *gin.Context, info models.User) {
	if info.Token == "" {
		generation, err := uuid.NewRandom()
		if err != nil {
			apiReturn.ErrorDatabase(c, "")
			return
		}
		result := global.Db.Model(&models.User{}).Where("id = ? AND password = ? AND status = 1 AND (token = '' OR token IS NULL)", info.ID, info.Password).
			UpdateColumn("token", strings.ReplaceAll(generation.String(), "-", ""))
		if result.Error != nil {
			apiReturn.ErrorDatabase(c, "")
			return
		}
	}
	// The password predicate prevents reissuing a session after a concurrent reset.
	var current models.User
	if err := global.Db.Where("id = ? AND password = ? AND status = 1", info.ID, info.Password).First(&current).Error; err != nil || current.Token == "" {
		apiReturn.ErrorByCode(c, 1001)
		return
	}
	token, err := security.RandomPassword()
	if err != nil {
		apiReturn.ErrorDatabase(c, "")
		return
	}
	global.CUserToken.SetDefault(token, current.Token)
	c.Header("Cache-Control", "no-store")
	response := publicUser(current)
	response["token"] = token
	response["mustChangePassword"] = security.RequiresPasswordChange(current.Password)
	apiReturn.SuccessData(c, response)
}

// PasskeyLoginBegin 开始 Passkey 登录（public，免用户名 discoverable，强制 UV）
//
// @Summary 开始 Passkey 登录
// @Tags user
// @Router /login/passkey/begin [post]
func (l LoginApi) PasskeyLoginBegin(c *gin.Context) {
	result, err := biz.Security.BeginPasskeyLogin(biz.SessionBinding(c.GetHeader("token")))
	if err != nil {
		securityErrorResponse(c, err)
		return
	}

	apiReturn.SuccessData(c, gin.H{
		"flowId":    result.FlowID,
		"publicKey": result.PublicKey,
	})
}

// PasskeyLoginFinish 完成 Passkey 登录（public）
//
// @Summary 完成 Passkey 登录
// @Tags user
// @Router /login/passkey/finish [post]
func (l LoginApi) PasskeyLoginFinish(c *gin.Context) {
	param := securityCredentialParam{}
	if err := c.ShouldBindJSON(&param); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	if errMsg, err := base.ValidateInputStruct(param); err != nil {
		apiReturn.ErrorParamFomat(c, errMsg)
		return
	}

	user, err := biz.Security.FinishPasskeyLogin(
		param.FlowID,
		biz.SessionBinding(c.GetHeader("token")),
		param.Credential,
	)
	if err != nil {
		securityErrorResponse(c, err)
		return
	}

	l.issueSession(c, user)
}

// 安全退出
func (l *LoginApi) Logout(c *gin.Context) {
	// userInfo, _ := base.GetCurrentUserInfo(c)
	cToken := c.GetHeader("token")
	global.CUserToken.Delete(cToken)
	apiReturn.Success(c)
}
