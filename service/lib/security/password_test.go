package security

import (
	"sun-panel/lib/cmn"
	"testing"
	"time"
)

func TestPasswordLegacyAndArgon2id(t *testing.T) {
	const password = "local test password"
	legacy := cmn.PasswordEncryption(password)
	if ok, upgrade := VerifyPassword(password, legacy); !ok || !upgrade {
		t.Fatal("legacy password could not be migrated")
	}
	first, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("identical passwords reused a salt")
	}
	if ok, upgrade := VerifyPassword(password, first); !ok || upgrade {
		t.Fatal("Argon2id roundtrip failed")
	}
	if ok, _ := VerifyPassword("mismatch", first); ok {
		t.Fatal("a different password matched")
	}
	if ok, _ := VerifyPassword(password, "$argon2id$v=19$m=99999999,t=2,p=1$short$short"); ok {
		t.Fatal("unrecognized parameters accepted")
	}
	if !RequiresPasswordChange(cmn.PasswordEncryption(LegacyDefaultPassword)) || RequiresPasswordChange(legacy) {
		t.Fatal("original default detection failed")
	}
	if ValidateNewPassword(LegacyDefaultPassword) == nil {
		t.Fatal("original default still allowed for new passwords")
	}
}

func TestPasswordLimiterAccountAndSource(t *testing.T) {
	limiter := attemptLimiter{accounts: map[string]attempt{}, sources: map[string]attempt{}}
	now := time.Unix(1000, 0)
	for i := 0; i < 12; i++ {
		if !limiter.Allow("alice", "source-a", now) {
			t.Fatal("account limited too soon")
		}
	}
	if limiter.Allow(" ALICE ", "source-b", now) {
		t.Fatal("account limit missed equivalent account on another source")
	}
	for i := 0; i < 60; i++ {
		if !limiter.Allow(string(rune('A'+i)), "source-c", now) {
			t.Fatal("source limited too soon")
		}
	}
	if limiter.Allow("bob", "source-c", now) {
		t.Fatal("source limit did not apply across accounts")
	}
	if !limiter.Allow("alice", "source-a", now.Add(5*time.Minute)) || !limiter.Allow("bob", "source-c", now.Add(5*time.Minute)) {
		t.Fatal("limits did not expire")
	}
}
