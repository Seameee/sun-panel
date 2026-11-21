package biz

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"sun-panel/global"
	"sun-panel/lib/security"
	"sun-panel/models"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// SecurityType 二次认证（TOTP / Passkey）业务层
//
// 设计约束：
//   - 只读取旧 User 表的 id/username/name/status/password，不修改旧表结构与密码格式；
//   - 新增状态全部存放在独立的 security_* 表中，通过 user_id 关联；
//   - 一次性挑战只在内存中保存，且严格单次消费、绑定账号与当前会话。
type SecurityType struct {
	// Now 仅供测试注入时钟，生产环境保持 nil
	Now func() time.Time
}

// Security 二次认证业务实例
var Security = SecurityType{}

const (
	securityChallengeCapacity  = 4096
	securityMaxPasskeysPerUser = 20
	securityPasskeyNameMax     = 40
)

// 业务错误。上层 API 依据这些错误映射到既有业务码。
var (
	// ErrSecurityPassword 密码校验失败
	ErrSecurityPassword = errors.New("password verification failed")
	// ErrSecurityTotpRequired 已启用 TOTP 但未提供验证码（业务码 1101）
	ErrSecurityTotpRequired = errors.New("totp code required")
	// ErrSecurityTotpInvalid 验证码错误或重放（业务码 1102）
	ErrSecurityTotpInvalid = errors.New("totp code invalid or already used")
	// ErrSecurityTotpLocked 验证码错误次数过多，账号被临时锁定（业务码 1102）
	ErrSecurityTotpLocked = errors.New("totp verification temporarily locked")
	// ErrSecurityTotpAlreadyEnabled 已启用 TOTP，需先关闭再重新绑定
	ErrSecurityTotpAlreadyEnabled = errors.New("totp is already enabled")
	// ErrSecurityTotpNotEnabled 未启用 TOTP
	ErrSecurityTotpNotEnabled = errors.New("totp is not enabled")
	// ErrSecurityChallengeInvalid 挑战不存在、已过期、已被使用或与当前账号/会话不匹配
	ErrSecurityChallengeInvalid = errors.New("challenge invalid or expired")
	// ErrSecurityPasskeyUnavailable 未配置 WebAuthn 可信来源，Passkey 功能关闭
	ErrSecurityPasskeyUnavailable = errors.New("passkey is not configured")
	// ErrSecurityCredentialInvalid 凭据校验失败
	ErrSecurityCredentialInvalid = errors.New("passkey credential invalid")
	// ErrSecurityPasskeyExists 凭据已注册
	ErrSecurityPasskeyExists = errors.New("passkey credential already registered")
	// ErrSecurityPasskeyLimit 单个账号的 Passkey 数量超限
	ErrSecurityPasskeyLimit = errors.New("too many passkeys for this account")
	// ErrSecurityNotFound 未找到记录
	ErrSecurityNotFound = errors.New("record not found")
	// ErrSecurityParams 参数不合法
	ErrSecurityParams = errors.New("invalid parameters")
	// ErrSecurityAccountDisabled 账号已停用或未激活（业务码 1004）
	ErrSecurityAccountDisabled = errors.New("account disabled or not activated")
)

// SecurityConfig 二次认证配置
//
// 配置来源为 conf.ini 的 [security] 段（全部可选，缺省值见 defaultSecurityConfig）：
//
//	[security]
//	; WebAuthn 依赖方 ID，通常是站点域名（不带协议与端口），例如 panel.example.com
//	; 为空表示关闭 Passkey 功能（/login/passkey/* 与管理接口返回未配置错误）
//	webauthn_rp_id = panel.example.com
//	; 允许的浏览器 Origin，逗号分隔，必须是 https://，或 http://localhost / http://127.0.0.1 / http://[::1]
//	; 反向代理场景填写浏览器地址栏里的公网 Origin
//	webauthn_origins = https://panel.example.com
//	; WebAuthn 展示名称
//	webauthn_rp_name = Sun-Panel
//	; TOTP 发行方名称（显示在验证器 App 中）
//	totp_issuer = Sun-Panel
//	; 账号维度防爆破：连续错误次数上限与锁定时长（秒）
//	totp_max_attempts = 5
//	totp_lock_seconds = 300
//	; 一次性挑战有效期（秒）
//	challenge_ttl_seconds = 120
type SecurityConfig struct {
	RPID             string
	RPName           string
	RPOrigins        []string
	TotpIssuer       string
	TotpMaxAttempts  int
	TotpLockDuration time.Duration
	TotpWindow       int
	ChallengeTTL     time.Duration
}

// PasskeyAvailable 判断 Passkey 功能是否可用
func (c SecurityConfig) PasskeyAvailable() bool {
	return c.RPID != "" && len(c.RPOrigins) > 0
}

// SecurityStatus 认证管理状态
type SecurityStatus struct {
	TotpEnabled      bool                  `json:"totpEnabled"`
	Passkeys         []SecurityPasskeyInfo `json:"passkeys"`
	PasskeyAvailable bool                  `json:"passkeyAvailable"`
}

// SecurityPasskeyInfo Passkey 展示信息（不包含公钥与凭据 ID）
type SecurityPasskeyInfo struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// SecurityTotpEnrollResult TOTP 绑定开始结果
type SecurityTotpEnrollResult struct {
	FlowID     string `json:"flowId"`
	Secret     string `json:"secret"`
	OtpauthURL string `json:"otpauthUrl"`
}

// SecurityPasskeyBeginResult Passkey 注册/登录开始结果
type SecurityPasskeyBeginResult struct {
	FlowID    string      `json:"flowId"`
	PublicKey interface{} `json:"publicKey"`
}

var (
	securityChallengeStoreOnce sync.Once
	securityChallengeStore     *security.ChallengeStore
)

func challenges() *security.ChallengeStore {
	securityChallengeStoreOnce.Do(func() {
		securityChallengeStore = security.NewChallengeStore(2*time.Minute, securityChallengeCapacity)
	})
	return securityChallengeStore
}

func (s SecurityType) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func securityDb() *gorm.DB {
	if models.Db != nil {
		return models.Db.Session(&gorm.Session{Logger: logger.Discard})
	}
	return global.Db.Session(&gorm.Session{Logger: logger.Discard})
}

// SessionBinding 将当前会话 token 转换为挑战绑定值（不保存原文）
func SessionBinding(cToken string) string {
	if cToken == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(cToken))
	return hex.EncodeToString(sum[:])
}

func defaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		RPName:           "Sun-Panel",
		TotpIssuer:       "Sun-Panel",
		TotpMaxAttempts:  5,
		TotpLockDuration: 5 * time.Minute,
		TotpWindow:       security.TotpDefaultWindow,
		ChallengeTTL:     2 * time.Minute,
	}
}

// Config 读取并校验 [security] 配置；非法值会被丢弃而不是放行
func (s SecurityType) Config() SecurityConfig {
	cfg := defaultSecurityConfig()
	if global.Config == nil || global.Config.Config == nil {
		return cfg
	}

	get := func(name string) string {
		return strings.TrimSpace(global.Config.GetValueString("security", name))
	}

	if v := strings.ToLower(get("webauthn_rp_id")); v != "" && !strings.ContainsAny(v, "/:?#") {
		cfg.RPID = v
	}
	if v := get("webauthn_rp_name"); v != "" {
		cfg.RPName = v
	}
	if v := get("totp_issuer"); v != "" {
		cfg.TotpIssuer = v
	}
	if v := global.Config.GetValueInt("security", "totp_max_attempts"); v > 0 {
		cfg.TotpMaxAttempts = v
	}
	if v := global.Config.GetValueInt("security", "totp_lock_seconds"); v > 0 {
		cfg.TotpLockDuration = time.Duration(v) * time.Second
	}
	if v := global.Config.GetValueInt("security", "challenge_ttl_seconds"); v > 0 {
		cfg.ChallengeTTL = time.Duration(v) * time.Second
	}

	for _, raw := range strings.Split(get("webauthn_origins"), ",") {
		origin, ok := normalizeWebAuthnOrigin(raw)
		if !ok {
			continue
		}
		if cfg.RPID == "" || !originMatchesRPID(origin, cfg.RPID) {
			continue
		}
		cfg.RPOrigins = append(cfg.RPOrigins, origin)
	}

	return cfg
}

func normalizeWebAuthnOrigin(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	if u.Path != "" && u.Path != "/" {
		return "", false
	}

	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	hostname := strings.ToLower(u.Hostname())

	switch scheme {
	case "https":
	case "http":
		// 仅允许本机开发环境使用明文 http
		if hostname != "localhost" && hostname != "127.0.0.1" && hostname != "::1" {
			return "", false
		}
	default:
		return "", false
	}

	return scheme + "://" + host, true
}

func originMatchesRPID(origin, rpID string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	hostname := strings.ToLower(u.Hostname())
	return hostname == rpID || strings.HasSuffix(hostname, "."+rpID)
}

func (s SecurityType) webAuthn(cfg SecurityConfig) (*webauthn.WebAuthn, error) {
	if !cfg.PasskeyAvailable() {
		return nil, ErrSecurityPasskeyUnavailable
	}

	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.RPID,
		RPDisplayName: cfg.RPName,
		RPOrigins:     cfg.RPOrigins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			RequireResidentKey: protocol.ResidentKeyRequired(),
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			UserVerification:   protocol.VerificationRequired,
		},
		AttestationPreference: protocol.PreferNoAttestation,
	})
	if err != nil {
		return nil, ErrSecurityPasskeyUnavailable
	}
	return wa, nil
}

func getUser(userID uint) (models.User, error) {
	var user models.User
	err := securityDb().First(&user, "id = ?", userID).Error
	return user, err
}

// VerifyPassword 使用旧格式（三重 MD5）校验密码，不做任何额外整改
func (s SecurityType) VerifyPassword(userID uint, password string) (models.User, error) {
	user, err := getUser(userID)
	if err != nil {
		return user, err
	}
	if password == "" {
		return user, ErrSecurityPassword
	}
	if valid, _ := security.VerifyPassword(password, user.Password); !valid {
		return user, ErrSecurityPassword
	}
	return user, nil
}

// TotpEnabled 判断账号是否已启用 TOTP
func (s SecurityType) TotpEnabled(userID uint) (bool, error) {
	var rec models.SecurityTotp
	err := securityDb().Where("user_id = ?", userID).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return rec.Enabled, nil
}

// Status 返回认证管理状态（不包含任何密钥/公钥/凭据 ID）
func (s SecurityType) Status(userID uint) (SecurityStatus, error) {
	cfg := s.Config()
	status := SecurityStatus{
		Passkeys:         []SecurityPasskeyInfo{},
		PasskeyAvailable: cfg.PasskeyAvailable(),
	}

	var totp models.SecurityTotp
	err := securityDb().Where("user_id = ?", userID).First(&totp).Error
	if err == nil {
		status.TotpEnabled = totp.Enabled
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return status, err
	}

	var passkeys []models.SecurityPasskey
	if err := securityDb().Where("user_id = ?", userID).Order("id asc").Find(&passkeys).Error; err != nil {
		return status, err
	}
	for _, item := range passkeys {
		status.Passkeys = append(status.Passkeys, SecurityPasskeyInfo{
			ID:        item.ID,
			Name:      item.Name,
			CreatedAt: item.CreatedAt,
		})
	}

	return status, nil
}

// VerifyLoginTotp 登录阶段校验 TOTP。
//
// 返回值语义：
//   - nil：未启用 TOTP（兼容原密码登录）或验证通过；
//   - ErrSecurityTotpRequired：已启用但未提交验证码（不得下发 token）；
//   - ErrSecurityTotpInvalid / ErrSecurityTotpLocked：错误、重放或已锁定。
//
// 防重放使用持久化 CAS：只有把 last_used_step 从更小值推进到本次时间步的那一次请求算成功，
// 同一个验证码（同一时间步）第二次提交时 RowsAffected 为 0。
func (s SecurityType) VerifyLoginTotp(userID uint, code string) error {
	cfg := s.Config()

	var rec models.SecurityTotp
	err := securityDb().Where("user_id = ?", userID).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !rec.Enabled {
		return nil
	}

	now := s.now()
	if rec.LockedUntil != nil && now.Before(*rec.LockedUntil) {
		return ErrSecurityTotpLocked
	}
	if security.NormalizeTotpCode(code) == "" {
		return ErrSecurityTotpRequired
	}

	step, matchErr := security.MatchTotpCode(rec.Secret, code, now, cfg.TotpWindow)
	if matchErr != nil {
		s.registerTotpFailure(rec.ID, cfg)
		return ErrSecurityTotpInvalid
	}

	res := securityDb().Model(&models.SecurityTotp{}).
		Where("id = ? AND enabled = ? AND secret = ? AND last_used_step < ? AND (locked_until IS NULL OR locked_until <= ?)", rec.ID, true, rec.Secret, step, now).
		Updates(map[string]interface{}{
			"last_used_step": step,
			"failed_count":   0,
			"locked_until":   nil,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		// 同一时间步已被消费：视为重放
		return ErrSecurityTotpInvalid
	}

	return nil
}

func (s SecurityType) registerTotpFailure(recordID uint, cfg SecurityConfig) {
	db := securityDb()
	if db == nil {
		return
	}

	if err := db.Model(&models.SecurityTotp{}).
		Where("id = ?", recordID).
		UpdateColumn("failed_count", gorm.Expr("failed_count + 1")).Error; err != nil {
		global.Logger.Debugln("security: update totp failed_count error:", err)
		return
	}

	if cfg.TotpMaxAttempts <= 0 {
		return
	}

	// 达到阈值则加锁并清零计数；条件更新保证并发下只锁一次
	if err := db.Model(&models.SecurityTotp{}).
		Where("id = ? AND failed_count >= ?", recordID, cfg.TotpMaxAttempts).
		Updates(map[string]interface{}{
			"failed_count": 0,
			"locked_until": s.now().Add(cfg.TotpLockDuration),
		}).Error; err != nil {
		global.Logger.Debugln("security: lock totp error:", err)
	}
}

// BeginTotpEnroll 开始 TOTP 绑定：校验密码，生成待确认密钥并返回一次性挑战
func (s SecurityType) BeginTotpEnroll(userID uint, password, session string) (SecurityTotpEnrollResult, error) {
	var result SecurityTotpEnrollResult
	cfg := s.Config()

	user, err := s.VerifyPassword(userID, password)
	if err != nil {
		return result, err
	}

	enabled, err := s.TotpEnabled(userID)
	if err != nil {
		return result, err
	}
	if enabled {
		return result, ErrSecurityTotpAlreadyEnabled
	}

	secret, err := security.GenerateTotpSecret()
	if err != nil {
		return result, err
	}

	flowID, err := security.NewChallengeID()
	if err != nil {
		return result, err
	}

	challenge := &security.Challenge{
		ID:        flowID,
		Kind:      security.ChallengeTotpEnroll,
		UserID:    userID,
		Session:   session,
		Values:    map[string]string{"secret": secret},
		ExpiresAt: s.now().Add(cfg.ChallengeTTL),
	}
	if err := challenges().Put(challenge); err != nil {
		return result, err
	}

	account := user.Username
	if account == "" {
		account = fmt.Sprintf("user-%d", userID)
	}

	result.FlowID = flowID
	result.Secret = secret
	result.OtpauthURL = security.TotpOtpauthURL(cfg.TotpIssuer, account, secret)
	return result, nil
}

// FinishTotpEnroll 确认 TOTP 绑定
func (s SecurityType) FinishTotpEnroll(userID uint, flowID, code, session string) error {
	cfg := s.Config()

	challenge, err := challenges().Take(flowID, security.ChallengeTotpEnroll)
	if err != nil {
		return ErrSecurityChallengeInvalid
	}
	if !challenge.Matches(userID, session) {
		return ErrSecurityChallengeInvalid
	}

	secret := challenge.Values["secret"]
	if secret == "" {
		return ErrSecurityChallengeInvalid
	}

	step, matchErr := security.MatchTotpCode(secret, code, s.now(), cfg.TotpWindow)
	if matchErr != nil {
		return ErrSecurityTotpInvalid
	}

	now := s.now()
	return securityDb().Transaction(func(tx *gorm.DB) error {
		var rec models.SecurityTotp
		err := tx.Where("user_id = ?", userID).First(&rec).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&models.SecurityTotp{
				UserId:       userID,
				Secret:       secret,
				Enabled:      true,
				ConfirmedAt:  &now,
				LastUsedStep: step,
			}).Error
		}
		if err != nil {
			return err
		}

		res := tx.Model(&models.SecurityTotp{}).Where("id = ? AND enabled = ?", rec.ID, false).Updates(map[string]interface{}{
			"secret":         secret,
			"enabled":        true,
			"confirmed_at":   now,
			"last_used_step": step,
			"failed_count":   0,
			"locked_until":   nil,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrSecurityChallengeInvalid
		}
		return nil
	})
}

// DisableTotp 关闭 TOTP：需要密码 + 当前有效验证码
func (s SecurityType) DisableTotp(userID uint, password, code string) error {
	if _, err := s.VerifyPassword(userID, password); err != nil {
		return err
	}

	enabled, err := s.TotpEnabled(userID)
	if err != nil {
		return err
	}
	if !enabled {
		return ErrSecurityTotpNotEnabled
	}

	if err := s.VerifyLoginTotp(userID, code); err != nil {
		return err
	}

	return securityDb().Where("user_id = ?", userID).Delete(&models.SecurityTotp{}).Error
}

func (s SecurityType) requireSecondFactor(userID uint, totpCode string) error {
	enabled, err := s.TotpEnabled(userID)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	return s.VerifyLoginTotp(userID, totpCode)
}

// BeginPasskeyRegister 开始 Passkey 注册：需要密码 +（已启用时）TOTP
func (s SecurityType) BeginPasskeyRegister(userID uint, password, totpCode, name, session string) (SecurityPasskeyBeginResult, error) {
	var result SecurityPasskeyBeginResult
	cfg := s.Config()

	user, err := s.VerifyPassword(userID, password)
	if err != nil {
		return result, err
	}
	if err := s.requireSecondFactor(userID, totpCode); err != nil {
		return result, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = "Passkey"
	}
	if len([]rune(name)) > securityPasskeyNameMax {
		return result, ErrSecurityParams
	}

	wa, err := s.webAuthn(cfg)
	if err != nil {
		return result, err
	}

	credentials, err := s.loadWebAuthnCredentials(userID)
	if err != nil {
		return result, err
	}
	if len(credentials) >= securityMaxPasskeysPerUser {
		return result, ErrSecurityPasskeyLimit
	}

	exclusions := make([]protocol.CredentialDescriptor, 0, len(credentials))
	for _, credential := range credentials {
		exclusions = append(exclusions, credential.Descriptor())
	}

	creation, sessionData, err := wa.BeginRegistration(
		webAuthnUser{user: user, credentials: credentials},
		webauthn.WithExclusions(exclusions),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			RequireResidentKey: protocol.ResidentKeyRequired(),
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			UserVerification:   protocol.VerificationRequired,
		}),
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
	)
	if err != nil {
		return result, ErrSecurityPasskeyUnavailable
	}

	flowID, err := security.NewChallengeID()
	if err != nil {
		return result, err
	}

	challenge := &security.Challenge{
		ID:          flowID,
		Kind:        security.ChallengePasskeyEnroll,
		UserID:      userID,
		Session:     session,
		SessionData: *sessionData,
		Values:      map[string]string{"name": name},
		ExpiresAt:   s.now().Add(cfg.ChallengeTTL),
	}
	if err := challenges().Put(challenge); err != nil {
		return result, err
	}

	result.FlowID = flowID
	result.PublicKey = creation.Response
	return result, nil
}

// FinishPasskeyRegister 完成 Passkey 注册
func (s SecurityType) FinishPasskeyRegister(userID uint, flowID, session string, credentialJSON []byte) error {
	cfg := s.Config()

	wa, err := s.webAuthn(cfg)
	if err != nil {
		return err
	}

	challenge, err := challenges().Take(flowID, security.ChallengePasskeyEnroll)
	if err != nil {
		return ErrSecurityChallengeInvalid
	}
	if !challenge.Matches(userID, session) {
		return ErrSecurityChallengeInvalid
	}
	if len(credentialJSON) == 0 {
		return ErrSecurityParams
	}

	user, err := getUser(userID)
	if err != nil {
		return err
	}
	credentials, err := s.loadWebAuthnCredentials(userID)
	if err != nil {
		return err
	}

	// WebAuthn 库要求先解析客户端注册响应
	parsedCreation, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(credentialJSON))
	if err != nil {
		return ErrSecurityCredentialInvalid
	}

	credential, err := wa.CreateCredential(
		webAuthnUser{user: user, credentials: credentials},
		challenge.SessionData,
		parsedCreation,
	)
	if err != nil {
		return ErrSecurityCredentialInvalid
	}

	credentialID := base64.RawURLEncoding.EncodeToString(credential.ID)
	var exists models.SecurityPasskey
	if err := securityDb().Where("credential_id = ?", credentialID).First(&exists).Error; err == nil {
		return ErrSecurityPasskeyExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	name := challenge.Values["name"]
	if name == "" {
		name = "Passkey"
	}

	record := models.SecurityPasskey{
		UserId:          userID,
		CredentialID:    credentialID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		AAGUID:          hex.EncodeToString(credential.Authenticator.AAGUID),
		SignCount:       credential.Authenticator.SignCount,
		CloneWarning:    credential.Authenticator.CloneWarning,
		Transports:      joinTransports(credential.Transport),
		BackupEligible:  credential.Flags.BackupEligible,
		BackupState:     credential.Flags.BackupState,
		UserPresent:     credential.Flags.UserPresent,
		UserVerified:    credential.Flags.UserVerified,
		Name:            name,
	}
	if err := securityDb().Create(&record).Error; err != nil {
		return err
	}

	return nil
}

// BeginPasskeyLogin 开始免用户名（discoverable）Passkey 登录，强制用户验证（UV）
func (s SecurityType) BeginPasskeyLogin(session string) (SecurityPasskeyBeginResult, error) {
	var result SecurityPasskeyBeginResult
	cfg := s.Config()

	wa, err := s.webAuthn(cfg)
	if err != nil {
		return result, err
	}

	assertion, sessionData, err := wa.BeginDiscoverableLogin(
		webauthn.WithUserVerification(protocol.VerificationRequired),
	)
	if err != nil {
		return result, ErrSecurityPasskeyUnavailable
	}

	flowID, err := security.NewChallengeID()
	if err != nil {
		return result, err
	}

	challenge := &security.Challenge{
		ID:          flowID,
		Kind:        security.ChallengePasskeyLogin,
		Session:     session,
		SessionData: *sessionData,
		ExpiresAt:   s.now().Add(cfg.ChallengeTTL),
	}
	if err := challenges().Put(challenge); err != nil {
		return result, err
	}

	result.FlowID = flowID
	result.PublicKey = assertion.Response
	return result, nil
}

// FinishPasskeyLogin 完成 Passkey 登录，成功返回凭据所属账号
func (s SecurityType) FinishPasskeyLogin(flowID, session string, credentialJSON []byte) (models.User, error) {
	var user models.User
	cfg := s.Config()

	wa, err := s.webAuthn(cfg)
	if err != nil {
		return user, err
	}

	challenge, err := challenges().Take(flowID, security.ChallengePasskeyLogin)
	if err != nil {
		return user, ErrSecurityChallengeInvalid
	}
	// 挑战与发起时的会话绑定，防止跨会话搬运 flowId
	if challenge.Session != session {
		return user, ErrSecurityChallengeInvalid
	}
	if len(credentialJSON) == 0 {
		return user, ErrSecurityParams
	}

	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		credentialID := base64.RawURLEncoding.EncodeToString(rawID)
		var passkey models.SecurityPasskey
		if err := securityDb().Where("credential_id = ?", credentialID).First(&passkey).Error; err != nil {
			return nil, ErrSecurityCredentialInvalid
		}

		owner, err := getUser(passkey.UserId)
		if err != nil {
			return nil, ErrSecurityCredentialInvalid
		}
		credentials, err := s.loadWebAuthnCredentials(passkey.UserId)
		if err != nil {
			return nil, err
		}
		return webAuthnUser{user: owner, credentials: credentials}, nil
	}

	credential, err := wa.FinishDiscoverableLogin(
		handler,
		challenge.SessionData,
		newCredentialRequest(credentialJSON),
	)
	if err != nil {
		return user, ErrSecurityCredentialInvalid
	}

	credentialID := base64.RawURLEncoding.EncodeToString(credential.ID)
	var passkey models.SecurityPasskey
	if err := securityDb().Where("credential_id = ?", credentialID).First(&passkey).Error; err != nil {
		return user, ErrSecurityCredentialInvalid
	}

	now := s.now()
	if err := securityDb().Model(&models.SecurityPasskey{}).Where("id = ?", passkey.ID).
		Updates(map[string]interface{}{
			"sign_count":      credential.Authenticator.SignCount,
			"clone_warning":   credential.Authenticator.CloneWarning,
			"backup_eligible": credential.Flags.BackupEligible,
			"backup_state":    credential.Flags.BackupState,
			"user_present":    credential.Flags.UserPresent,
			"user_verified":   credential.Flags.UserVerified,
			"last_used_at":    now,
		}).Error; err != nil {
		return user, err
	}

	if credential.Authenticator.CloneWarning {
		global.Logger.Warnln("security: passkey sign count did not increase, possible cloned authenticator, user:", passkey.UserId)
	}

	user, err = getUser(passkey.UserId)
	if err != nil {
		return user, err
	}
	if user.Status != 1 {
		return user, ErrSecurityAccountDisabled
	}

	return user, nil
}

// DeletePasskey 删除 Passkey：需要密码 +（已启用时）TOTP
func (s SecurityType) DeletePasskey(userID, passkeyID uint, password, totpCode string) error {
	if _, err := s.VerifyPassword(userID, password); err != nil {
		return err
	}
	if err := s.requireSecondFactor(userID, totpCode); err != nil {
		return err
	}

	res := securityDb().Where("id = ? AND user_id = ?", passkeyID, userID).Delete(&models.SecurityPasskey{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrSecurityNotFound
	}
	return nil
}

func (s SecurityType) loadWebAuthnCredentials(userID uint) ([]webauthn.Credential, error) {
	var records []models.SecurityPasskey
	if err := securityDb().Where("user_id = ?", userID).Order("id asc").Find(&records).Error; err != nil {
		return nil, err
	}

	credentials := make([]webauthn.Credential, 0, len(records))
	for _, record := range records {
		credential, err := recordToWebAuthnCredential(record)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	return credentials, nil
}

func recordToWebAuthnCredential(record models.SecurityPasskey) (webauthn.Credential, error) {
	credentialID, err := base64.RawURLEncoding.DecodeString(record.CredentialID)
	if err != nil {
		return webauthn.Credential{}, ErrSecurityCredentialInvalid
	}

	aaguid, err := hex.DecodeString(record.AAGUID)
	if err != nil {
		aaguid = nil
	}

	return webauthn.Credential{
		ID:              credentialID,
		PublicKey:       record.PublicKey,
		AttestationType: record.AttestationType,
		Transport:       splitTransports(record.Transports),
		Flags: webauthn.CredentialFlags{
			UserPresent:    record.UserPresent,
			UserVerified:   record.UserVerified,
			BackupEligible: record.BackupEligible,
			BackupState:    record.BackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:       aaguid,
			SignCount:    record.SignCount,
			CloneWarning: record.CloneWarning,
		},
	}, nil
}

func joinTransports(transports []protocol.AuthenticatorTransport) string {
	if len(transports) == 0 {
		return ""
	}
	parts := make([]string, 0, len(transports))
	for _, transport := range transports {
		parts = append(parts, string(transport))
	}
	return strings.Join(parts, ",")
}

func splitTransports(raw string) []protocol.AuthenticatorTransport {
	if raw == "" {
		return nil
	}
	transports := make([]protocol.AuthenticatorTransport, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			transports = append(transports, protocol.AuthenticatorTransport(part))
		}
	}
	return transports
}

func newCredentialRequest(credentialJSON []byte) *http.Request {
	req, _ := http.NewRequest(http.MethodPost, "/", bytes.NewReader(credentialJSON))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// webAuthnUser 把旧的 User 表适配为 webauthn.User
type webAuthnUser struct {
	user        models.User
	credentials []webauthn.Credential
}

func (u webAuthnUser) WebAuthnID() []byte {
	// 稳定的不透明 user handle（<= 64 字节），不直接暴露自增 ID 之外的敏感信息
	return []byte(fmt.Sprintf("sun-panel-user-%d", u.user.ID))
}

func (u webAuthnUser) WebAuthnName() string {
	if u.user.Username != "" {
		return u.user.Username
	}
	return fmt.Sprintf("user-%d", u.user.ID)
}

func (u webAuthnUser) WebAuthnDisplayName() string {
	if u.user.Name != "" {
		return u.user.Name
	}
	return u.WebAuthnName()
}

func (u webAuthnUser) WebAuthnIcon() string {
	return ""
}

func (u webAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}
