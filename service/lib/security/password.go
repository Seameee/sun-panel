package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"sun-panel/lib/cmn"

	"golang.org/x/crypto/argon2"
)

const (
	LegacyDefaultPassword = "12345678"
	MaxPasswordBytes      = 200
	argonPrefix           = "$argon2id$v=19$m=19456,t=2,p=1$"
)

var ErrPasswordPolicy = errors.New("password must be 8–200 bytes and differ from the original default")

// Bound concurrent memory use across login, account management and reauthentication.
var passwordWorkers = make(chan struct{}, 4)

func derivePassword(password string, salt []byte) []byte {
	passwordWorkers <- struct{}{}
	defer func() { <-passwordWorkers }()
	return argon2.IDKey([]byte(password), salt, 2, 19456, 1, 32)
}

func ValidateNewPassword(password string) error {
	if len(password) < 8 || len(password) > MaxPasswordBytes || password == LegacyDefaultPassword {
		return ErrPasswordPolicy
	}
	return nil
}

func HashPassword(password string) (string, error) {
	var salt [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", err
	}
	return argonPrefix + base64.RawStdEncoding.EncodeToString(salt[:]) + "$" + base64.RawStdEncoding.EncodeToString(derivePassword(password, salt[:])), nil
}

// VerifyPassword preserves the exact v1.3 password format and signals when it can be upgraded.
func VerifyPassword(password, encoded string) (valid, legacy bool) {
	if len(password) > MaxPasswordBytes {
		return false, false
	}
	if len(encoded) == 32 {
		return subtle.ConstantTimeCompare([]byte(cmn.PasswordEncryption(password)), []byte(encoded)) == 1, true
	}
	data, ok := strings.CutPrefix(encoded, argonPrefix)
	if !ok {
		return false, false
	}
	saltText, hashText, ok := strings.Cut(data, "$")
	if !ok || len(saltText) != 22 || len(hashText) != 43 {
		return false, false
	}
	salt, saltErr := base64.RawStdEncoding.Strict().DecodeString(saltText)
	hash, hashErr := base64.RawStdEncoding.Strict().DecodeString(hashText)
	if saltErr != nil || hashErr != nil || len(salt) != 16 || len(hash) != 32 {
		return false, false
	}
	return subtle.ConstantTimeCompare(derivePassword(password, salt), hash) == 1, false
}

func RequiresPasswordChange(hash string) bool {
	return subtle.ConstantTimeCompare([]byte(hash), []byte(cmn.PasswordEncryption(LegacyDefaultPassword))) == 1
}

func RandomPassword() (string, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}
