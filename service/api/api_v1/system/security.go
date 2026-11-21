package system

import (
	"encoding/json"
	"errors"
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/biz"

	"github.com/gin-gonic/gin"
)

// SecurityApi 二次认证管理接口（TOTP / Passkey）
//
// 全部接口都要求已登录（LoginInterceptor），并且：
//   - 涉及状态变更时重新校验密码（旧格式哈希，不做额外整改）；
//   - 已启用 TOTP 时，添加/删除 Passkey 必须再提交一个当前有效的 TOTP 验证码；
//   - 响应中不包含 TOTP 密钥、Passkey 公钥或凭据 ID。
type SecurityApi struct{}

// securityCredentialParam 客户端凭据注册/断言响应
type securityCredentialParam struct {
	FlowID     string          `json:"flowId" validate:"required"`
	Credential json.RawMessage `json:"credential" validate:"required"`
}

// securityErrorResponse 把业务错误映射到既有业务码与响应 envelope
func securityErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, biz.ErrSecurityPassword):
		apiReturn.ErrorByCode(c, apiReturn.ErrCodeOldPasswordError)
	case errors.Is(err, biz.ErrSecurityAccountDisabled):
		apiReturn.ErrorByCode(c, apiReturn.ErrCodeAccountDisabledOrNotActivated)
	case errors.Is(err, biz.ErrSecurityTotpRequired):
		apiReturn.ErrorByCode(c, apiReturn.ERROR_CODE_VERIFICATION_MUST)
	case errors.Is(err, biz.ErrSecurityTotpInvalid),
		errors.Is(err, biz.ErrSecurityTotpLocked),
		errors.Is(err, biz.ErrSecurityChallengeInvalid),
		errors.Is(err, biz.ErrSecurityCredentialInvalid):
		apiReturn.ErrorByCode(c, apiReturn.ERROR_CODE_VERIFICATION_FAIL)
	case errors.Is(err, biz.ErrSecurityPasskeyUnavailable),
		errors.Is(err, biz.ErrSecurityTotpAlreadyEnabled),
		errors.Is(err, biz.ErrSecurityTotpNotEnabled),
		errors.Is(err, biz.ErrSecurityPasskeyExists),
		errors.Is(err, biz.ErrSecurityPasskeyLimit),
		errors.Is(err, biz.ErrSecurityNotFound),
		errors.Is(err, biz.ErrSecurityParams):
		apiReturn.Error(c, err.Error())
	default:
		apiReturn.ErrorDatabase(c, err.Error())
	}
}

// Status 查询当前账号的二次认证状态
//
// @Summary 二次认证状态
// @Tags user
// @Router /user/security/status [post]
func (a SecurityApi) Status(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)

	status, err := biz.Security.Status(userInfo.ID)
	if err != nil {
		securityErrorResponse(c, err)
		return
	}

	apiReturn.SuccessData(c, status)
}

// TotpBegin 开始绑定 TOTP（返回一次性挑战与密钥，密钥只在此处下发）
//
// @Summary 开始绑定 TOTP
// @Tags user
// @Router /user/security/totp/begin [post]
func (a SecurityApi) TotpBegin(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)

	type paramStruct struct {
		Password string `json:"password" validate:"required"`
	}
	param := paramStruct{}
	if err := c.ShouldBindJSON(&param); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	if errMsg, err := base.ValidateInputStruct(param); err != nil {
		apiReturn.ErrorParamFomat(c, errMsg)
		return
	}

	result, err := biz.Security.BeginTotpEnroll(
		userInfo.ID,
		param.Password,
		biz.SessionBinding(c.GetHeader("token")),
	)
	if err != nil {
		securityErrorResponse(c, err)
		return
	}

	apiReturn.SuccessData(c, result)
}

// TotpFinish 提交验证码完成 TOTP 绑定
//
// @Summary 完成绑定 TOTP
// @Tags user
// @Router /user/security/totp/finish [post]
func (a SecurityApi) TotpFinish(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)

	type paramStruct struct {
		FlowID string `json:"flowId" validate:"required"`
		Code   string `json:"code" validate:"required,max=16"`
	}
	param := paramStruct{}
	if err := c.ShouldBindJSON(&param); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	if errMsg, err := base.ValidateInputStruct(param); err != nil {
		apiReturn.ErrorParamFomat(c, errMsg)
		return
	}

	if err := biz.Security.FinishTotpEnroll(
		userInfo.ID,
		param.FlowID,
		param.Code,
		biz.SessionBinding(c.GetHeader("token")),
	); err != nil {
		securityErrorResponse(c, err)
		return
	}

	apiReturn.Success(c)
}

// TotpDisable 关闭 TOTP（需要密码 + 当前有效验证码）
//
// @Summary 关闭 TOTP
// @Tags user
// @Router /user/security/totp/disable [post]
func (a SecurityApi) TotpDisable(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)

	type paramStruct struct {
		Password string `json:"password" validate:"required"`
		Code     string `json:"code" validate:"required,max=16"`
	}
	param := paramStruct{}
	if err := c.ShouldBindJSON(&param); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	if errMsg, err := base.ValidateInputStruct(param); err != nil {
		apiReturn.ErrorParamFomat(c, errMsg)
		return
	}

	if err := biz.Security.DisableTotp(userInfo.ID, param.Password, param.Code); err != nil {
		securityErrorResponse(c, err)
		return
	}

	apiReturn.Success(c)
}

// PasskeyBegin 开始注册 Passkey（需要密码，已启用 TOTP 时还需要 TOTP 验证码）
//
// @Summary 开始注册 Passkey
// @Tags user
// @Router /user/security/passkey/begin [post]
func (a SecurityApi) PasskeyBegin(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)

	type paramStruct struct {
		Password string `json:"password" validate:"required"`
		TotpCode string `json:"totpCode" validate:"max=16"`
		Name     string `json:"name" validate:"max=40"`
	}
	param := paramStruct{}
	if err := c.ShouldBindJSON(&param); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	if errMsg, err := base.ValidateInputStruct(param); err != nil {
		apiReturn.ErrorParamFomat(c, errMsg)
		return
	}

	result, err := biz.Security.BeginPasskeyRegister(
		userInfo.ID,
		param.Password,
		param.TotpCode,
		param.Name,
		biz.SessionBinding(c.GetHeader("token")),
	)
	if err != nil {
		securityErrorResponse(c, err)
		return
	}

	apiReturn.SuccessData(c, gin.H{
		"flowId":    result.FlowID,
		"publicKey": result.PublicKey,
	})
}

// PasskeyFinish 提交客户端凭据完成 Passkey 注册
//
// @Summary 完成注册 Passkey
// @Tags user
// @Router /user/security/passkey/finish [post]
func (a SecurityApi) PasskeyFinish(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)

	param := securityCredentialParam{}
	if err := c.ShouldBindJSON(&param); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	if errMsg, err := base.ValidateInputStruct(param); err != nil {
		apiReturn.ErrorParamFomat(c, errMsg)
		return
	}

	if err := biz.Security.FinishPasskeyRegister(
		userInfo.ID,
		param.FlowID,
		biz.SessionBinding(c.GetHeader("token")),
		param.Credential,
	); err != nil {
		securityErrorResponse(c, err)
		return
	}

	apiReturn.Success(c)
}

// PasskeyDelete 删除 Passkey（需要密码，已启用 TOTP 时还需要 TOTP 验证码）
//
// @Summary 删除 Passkey
// @Tags user
// @Router /user/security/passkey/delete [post]
func (a SecurityApi) PasskeyDelete(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)

	type paramStruct struct {
		ID       uint   `json:"id" validate:"required"`
		Password string `json:"password" validate:"required"`
		TotpCode string `json:"totpCode" validate:"max=16"`
	}
	param := paramStruct{}
	if err := c.ShouldBindJSON(&param); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	if errMsg, err := base.ValidateInputStruct(param); err != nil {
		apiReturn.ErrorParamFomat(c, errMsg)
		return
	}

	if err := biz.Security.DeletePasskey(userInfo.ID, param.ID, param.Password, param.TotpCode); err != nil {
		securityErrorResponse(c, err)
		return
	}

	apiReturn.Success(c)
}
