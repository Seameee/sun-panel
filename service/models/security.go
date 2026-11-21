package models

import "time"

// SecurityTotp 用户 TOTP 绑定信息
//
// 该表与旧的 User 表完全隔离，只通过 user_id 关联，不修改 User 表结构与密码格式。
type SecurityTotp struct {
	BaseModel
	UserId uint `gorm:"uniqueIndex;not null" json:"-"`
	// Secret 为 base32(无填充) 编码的 TOTP 共享密钥，仅在下发绑定二维码时读取，任何管理接口都不返回
	Secret      string     `gorm:"type:varchar(64);not null" json:"-"`
	Enabled     bool       `gorm:"not null;default:false" json:"-"`
	ConfirmedAt *time.Time `json:"-"`
	// LastUsedStep 为最后一次成功消费的 TOTP 时间步，用于持久化 CAS 防重放
	LastUsedStep int64 `gorm:"not null;default:0" json:"-"`
	// FailedCount / LockedUntil 为账号维度的防爆破状态
	FailedCount int        `gorm:"not null;default:0" json:"-"`
	LockedUntil *time.Time `json:"-"`
}

// SecurityPasskey 用户 WebAuthn 凭据
//
// 该表与旧的 User 表完全隔离，只通过 user_id 关联。
type SecurityPasskey struct {
	BaseModel
	UserId uint `gorm:"index;not null" json:"-"`
	// CredentialID 为 base64url(raw) 编码的凭据 ID
	CredentialID    string     `gorm:"type:varchar(255);uniqueIndex;not null" json:"-"`
	PublicKey       []byte     `gorm:"not null" json:"-"`
	AttestationType string     `gorm:"type:varchar(64)" json:"-"`
	AAGUID          string     `gorm:"type:varchar(64)" json:"-"`
	SignCount       uint32     `gorm:"not null;default:0" json:"-"`
	CloneWarning    bool       `gorm:"not null;default:false" json:"-"`
	Transports      string     `gorm:"type:varchar(120)" json:"-"`
	BackupEligible  bool       `gorm:"not null;default:false" json:"-"`
	BackupState     bool       `gorm:"not null;default:false" json:"-"`
	UserPresent     bool       `gorm:"not null;default:false" json:"-"`
	UserVerified    bool       `gorm:"not null;default:false" json:"-"`
	Name            string     `gorm:"type:varchar(64)" json:"name"`
	LastUsedAt      *time.Time `json:"-"`
}
