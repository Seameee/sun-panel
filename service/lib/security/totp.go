// Package security 提供与业务无关的认证基础件（TOTP 计算、一次性挑战存储）。
package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// TotpDigits TOTP 验证码位数（RFC 6238 常见配置）
	TotpDigits = 6
	// TotpPeriod TOTP 时间步长
	TotpPeriod = 30 * time.Second
	// TotpSecretBytes 共享密钥熵（160 bit）
	TotpSecretBytes = 20
	// TotpDefaultWindow 允许的相邻时间步数量（前后各 1 步）
	TotpDefaultWindow = 1
)

var (
	// ErrInvalidTotpCode 验证码格式错误或与密钥不匹配
	ErrInvalidTotpCode = errors.New("invalid totp code")
	// ErrInvalidTotpSecret 密钥无法解码
	ErrInvalidTotpSecret = errors.New("invalid totp secret")
)

var base32NoPadding = base32.StdEncoding.WithPadding(base32.NoPadding)

// TotpStep 返回给定时间对应的时间步编号
func TotpStep(t time.Time) int64 {
	return t.Unix() / int64(TotpPeriod/time.Second)
}

// GenerateTotpSecret 生成随机 base32(无填充) 共享密钥
func GenerateTotpSecret() (string, error) {
	buf := make([]byte, TotpSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base32NoPadding.EncodeToString(buf), nil
}

// TotpCodeAt 计算指定时间步的动态验证码
func TotpCodeAt(secret string, step int64) (string, error) {
	key, err := decodeTotpSecret(secret)
	if err != nil {
		return "", err
	}

	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))

	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])

	return fmt.Sprintf("%0*d", TotpDigits, value%1_000_000), nil
}

// MatchTotpCode 校验验证码并返回命中的时间步。
//
// window 为允许的相邻时间步数量，0 表示只接受当前时间步。
// 未命中返回 ErrInvalidTotpCode；调用方还需自行做重放（时间步单调递增）检查。
func MatchTotpCode(secret, code string, now time.Time, window int) (int64, error) {
	code = NormalizeTotpCode(code)
	if len(code) != TotpDigits {
		return 0, ErrInvalidTotpCode
	}
	if window < 0 {
		window = 0
	}

	current := TotpStep(now)
	var matched int64 = -1
	for delta := -window; delta <= window; delta++ {
		step := current + int64(delta)
		if step < 0 {
			continue
		}
		expected, err := TotpCodeAt(secret, step)
		if err != nil {
			return 0, err
		}
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 && matched < step {
			matched = step
		}
	}

	if matched < 0 {
		return 0, ErrInvalidTotpCode
	}
	return matched, nil
}

// NormalizeTotpCode 去除空格等分隔符
func NormalizeTotpCode(code string) string {
	return strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(code))
}

// IsValidTotpCodeFormat 判断是否为 6 位数字
func IsValidTotpCodeFormat(code string) bool {
	code = NormalizeTotpCode(code)
	if len(code) != TotpDigits {
		return false
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return true
}

func decodeTotpSecret(secret string) ([]byte, error) {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	if normalized == "" {
		return nil, ErrInvalidTotpSecret
	}
	key, err := base32NoPadding.DecodeString(strings.TrimRight(normalized, "="))
	if err != nil || len(key) == 0 {
		return nil, ErrInvalidTotpSecret
	}
	return key, nil
}

// TotpOtpauthURL 生成标准的 otpauth:// 绑定链接
func TotpOtpauthURL(issuer, account, secret string) string {
	if issuer == "" {
		issuer = "Sun-Panel"
	}
	label := issuer + ":" + account
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", fmt.Sprintf("%d", TotpDigits))
	query.Set("period", fmt.Sprintf("%d", int(TotpPeriod/time.Second)))

	return "otpauth://totp/" + url.PathEscape(label) + "?" + query.Encode()
}
