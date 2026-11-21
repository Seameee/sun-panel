package security

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// ChallengeKind 一次性挑战的类型
type ChallengeKind string

const (
	// ChallengeTotpEnroll TOTP 绑定
	ChallengeTotpEnroll ChallengeKind = "totp_enroll"
	// ChallengePasskeyEnroll Passkey 注册
	ChallengePasskeyEnroll ChallengeKind = "passkey_enroll"
	// ChallengePasskeyLogin Passkey 免用户名登录
	ChallengePasskeyLogin ChallengeKind = "passkey_login"
)

var (
	// ErrChallengeNotFound 挑战不存在（已被使用或从未下发）
	ErrChallengeNotFound = errors.New("challenge not found")
	// ErrChallengeExpired 挑战已过期
	ErrChallengeExpired = errors.New("challenge expired")
	// ErrChallengeKindMismatch 挑战类型不匹配
	ErrChallengeKindMismatch = errors.New("challenge kind mismatch")
	// ErrChallengeStoreFull 挑战池已满且无法驱逐
	ErrChallengeStoreFull = errors.New("challenge store full")
)

// Challenge 一次性挑战
//
// 绑定关系：
//   - UserID 绑定账号
//   - Session 绑定发起挑战时的当前会话（cToken 的哈希；未登录时为空）
type Challenge struct {
	ID          string
	Kind        ChallengeKind
	UserID      uint
	Session     string
	SessionData webauthn.SessionData
	// Values 存放流程内的临时数据（如待确认的 TOTP 密钥、Passkey 名称）
	Values    map[string]string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Matches 判断挑战是否属于给定账号与会话
func (c *Challenge) Matches(userID uint, session string) bool {
	return c.UserID == userID && c.Session == session
}

// NewChallengeID 生成 128 bit 随机挑战 ID
func NewChallengeID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// ChallengeStore 并发安全的内存挑战池
//
// 单次使用：Take 会原子地移除条目，因此同一个 flowId 只能被消费一次。
// 容量上限：Put 时先清理过期条目，仍然超限则驱逐最快过期的条目。
type ChallengeStore struct {
	mu       sync.Mutex
	items    map[string]*Challenge
	ttl      time.Duration
	capacity int
	now      func() time.Time
}

// NewChallengeStore 创建挑战池；ttl<=0 时使用 2 分钟，capacity<=0 时使用 4096
func NewChallengeStore(ttl time.Duration, capacity int) *ChallengeStore {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	if capacity <= 0 {
		capacity = 4096
	}
	return &ChallengeStore{
		items:    make(map[string]*Challenge),
		ttl:      ttl,
		capacity: capacity,
		now:      time.Now,
	}
}

// Put 存入一个挑战并补齐 ID / 时间字段
func (s *ChallengeStore) Put(ch *Challenge) error {
	if ch == nil {
		return ErrChallengeNotFound
	}

	now := s.now()
	if ch.ID == "" {
		id, err := NewChallengeID()
		if err != nil {
			return err
		}
		ch.ID = id
	}
	if ch.CreatedAt.IsZero() {
		ch.CreatedAt = now
	}
	if ch.ExpiresAt.IsZero() {
		ch.ExpiresAt = now.Add(s.ttl)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneLocked(now)

	if len(s.items) >= s.capacity {
		s.evictSoonestLocked()
		if len(s.items) >= s.capacity {
			return ErrChallengeStoreFull
		}
	}

	s.items[ch.ID] = ch
	return nil
}

// Take 原子取出并删除挑战（单次使用）
func (s *ChallengeStore) Take(id string, kind ChallengeKind) (*Challenge, error) {
	if id == "" {
		return nil, ErrChallengeNotFound
	}

	s.mu.Lock()
	ch, ok := s.items[id]
	if ok {
		delete(s.items, id)
	}
	s.mu.Unlock()

	if !ok {
		return nil, ErrChallengeNotFound
	}
	if ch.Kind != kind {
		return nil, ErrChallengeKindMismatch
	}
	if !ch.ExpiresAt.IsZero() && !s.now().Before(ch.ExpiresAt) {
		return nil, ErrChallengeExpired
	}
	return ch, nil
}

// Len 返回当前条目数（含尚未清理的过期条目之外的条目）
func (s *ChallengeStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(s.now())
	return len(s.items)
}

func (s *ChallengeStore) pruneLocked(now time.Time) {
	for id, ch := range s.items {
		if !ch.ExpiresAt.IsZero() && !now.Before(ch.ExpiresAt) {
			delete(s.items, id)
		}
	}
}

func (s *ChallengeStore) evictSoonestLocked() {
	var victimID string
	var victimExpire time.Time
	for id, ch := range s.items {
		if victimID == "" || ch.ExpiresAt.Before(victimExpire) {
			victimID = id
			victimExpire = ch.ExpiresAt
		}
	}
	if victimID != "" {
		delete(s.items, victimID)
	}
}
