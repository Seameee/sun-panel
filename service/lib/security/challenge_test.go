package security

import (
	"errors"
	"testing"
	"time"
)

func TestChallengeStoreReservesManagementCapacity(t *testing.T) {
	s := NewChallengeStore(time.Minute, 4)
	now := time.Unix(100, 0)
	s.now = func() time.Time { return now }

	put := func(kind ChallengeKind, expiry time.Time) *Challenge {
		t.Helper()
		ch := &Challenge{Kind: kind, ExpiresAt: expiry}
		if err := s.Put(ch); err != nil {
			t.Fatalf("put %s: %v", kind, err)
		}
		return ch
	}
	full := func(kind ChallengeKind) {
		t.Helper()
		if err := s.Put(&Challenge{Kind: kind}); !errors.Is(err, ErrChallengeStoreFull) {
			t.Fatalf("put %s: want full, got %v", kind, err)
		}
	}
	take := func(ch *Challenge) {
		t.Helper()
		if got, err := s.Take(ch.ID, ch.Kind); err != nil || got != ch {
			t.Fatalf("take %s: err=%v, same challenge=%v", ch.Kind, err, got == ch)
		}
	}

	first := put(ChallengePasskeyLogin, time.Time{})
	put(ChallengePasskeyLogin, now.Add(time.Second))
	full(ChallengePasskeyLogin)
	totp := put(ChallengeTotpEnroll, time.Time{})
	passkey := put(ChallengePasskeyEnroll, time.Time{})
	full(ChallengePasskeyLogin)
	full(ChallengePasskeyEnroll)
	if s.Len() != 4 {
		t.Fatal("full store should preserve all four outstanding flows")
	}

	now = now.Add(time.Second)
	replacement := put(ChallengePasskeyLogin, time.Time{})
	full(ChallengePasskeyLogin)
	take(first)
	if _, err := s.Take(first.ID, first.Kind); !errors.Is(err, ErrChallengeNotFound) {
		t.Fatalf("consumed challenge should not be reusable: %v", err)
	}
	newFlow := put(ChallengePasskeyLogin, time.Time{})
	take(totp)
	take(passkey)
	take(replacement)
	take(newFlow)
}
