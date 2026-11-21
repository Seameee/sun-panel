package system

import (
	"errors"
	"strconv"
	"strings"
	"sun-panel/api/api_v1/common/apiData/panelApiStructs"
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/biz"
	"sun-panel/global"
	"sun-panel/lib/cmn"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type LoginApi struct {
}

// 登录输入验证
type LoginLoginVerify struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required,max=50"`
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

	settings := panelApiStructs.GlobalSettingSite{}
	biz.GlobalSetting.GetSiteStting(&settings)
	// 验证验证码
	if settings.GlobalSettingSiteSetting.IsEnableLoginCaptcha {
		if err := biz.Captcha.CaptchaVerifyHandle(c, param.VCode, true); err != nil {
			global.Logger.Errorln("Login error:", err.Error())
			apiReturn.ErrorByCode(c, apiReturn.ErrCodeCaptchaError)
			return
		}
	}

	mUser := models.User{}
	var (
		err  error
		info models.User
	)
	param.Username = strings.TrimSpace(param.Username)
	if info, err = mUser.GetUserInfoByUsernameAndPassword(param.Username, cmn.PasswordEncryption(param.Password)); err != nil {
		// 未找到记录 账号或密码错误
		if err == gorm.ErrRecordNotFound {
			apiReturn.ErrorByCode(c, 1003)
			return
		} else {
			// 未知错误
			apiReturn.Error(c, err.Error())
			return
		}

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

	info = l.issueSession(c, info)
	apiReturn.SuccessData(c, info)
}

// issueSession 生成/复用登录会话。
//
// 密码登录与 Passkey 登录共用，保证两者返回完全一致的 UserInfo。
func (l LoginApi) issueSession(c *gin.Context, info models.User) models.User {
	mUser := models.User{}
	bToken := info.Token
	if info.Token == "" {
		// 生成token
		buildTokenOver := false
		for !buildTokenOver {
			bToken = cmn.BuildRandCode(32, cmn.RAND_CODE_MODE2)
			if _, err := mUser.GetUserInfoByToken(bToken); err != nil {
				// 保存token
				mUser.UpdateUserInfoByUserId(info.ID, map[string]interface{}{
					"token": bToken,
				})
				buildTokenOver = true
			}
		}
		info.Token = bToken
	}

	info.Password = ""
	info.ReferralCode = ""

	cToken := uuid.NewString() + "-" + cmn.Md5(cmn.Md5("userId"+strconv.Itoa(int(info.ID))))
	global.CUserToken.SetDefault(cToken, bToken)

	// 设置当前用户信息
	c.Set("userInfo", info)
	info.Token = cToken // 重要 采用cToken,隐藏真实token
	return info
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

	info := l.issueSession(c, user)
	apiReturn.SuccessData(c, info)
}

// 安全退出
func (l *LoginApi) Logout(c *gin.Context) {
	// userInfo, _ := base.GetCurrentUserInfo(c)
	cToken := c.GetHeader("token")
	global.CUserToken.Delete(cToken)
	apiReturn.Success(c)
}
