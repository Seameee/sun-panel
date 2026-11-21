package safeurl

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	ok := []string{
		"http://example.com",
		"https://example.com/logo.png",
		"https://example.com:8443/a?b=c",
		"http://[2606:4700::1111]/",
	}
	for _, raw := range ok {
		if _, err := Parse(raw); err != nil {
			t.Errorf("Parse(%q) = %v, want nil", raw, err)
		}
	}

	bad := map[string]error{
		"":                              ErrInvalidURL,
		"   ":                           ErrInvalidURL,
		"ftp://example.com/a":           ErrSchemeNotAllowed,
		"file:///etc/passwd":            ErrSchemeNotAllowed,
		"javascript:alert(1)":           ErrSchemeNotAllowed,
		"gopher://example.com/":         ErrSchemeNotAllowed,
		"https://user:pass@example.com": ErrCredentials,
		"http:///nohost":                ErrInvalidURL,
		"http://example.com:0/":         ErrInvalidURL,
		"http://example.com:99999/":     ErrInvalidURL,
	}
	for raw, want := range bad {
		_, err := Parse(raw)
		if !errors.Is(err, want) {
			t.Errorf("Parse(%q) = %v, want %v", raw, err, want)
		}
	}
}

func TestIsPublicIP(t *testing.T) {
	blocked := []string{
		"0.0.0.0", "127.0.0.1", "127.8.9.10", "10.0.0.1", "172.16.0.1",
		"192.168.1.1", "169.254.169.254", "100.64.0.1", "192.0.0.1",
		"198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1",
		"240.0.0.1", "255.255.255.255",
		"::", "::1", "fc00::1", "fd12:3456::1", "fe80::1", "ff02::1",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", "64:ff9b::7f00:1",
		"2001:db8::1",
	}
	for _, s := range blocked {
		if IsPublicIP(netip.MustParseAddr(s)) {
			t.Errorf("IsPublicIP(%s) = true, want false", s)
		}
	}

	allowed := []string{"1.1.1.1", "8.8.8.8", "93.184.215.14", "2606:4700::1111", "2a00:1450:4001::1"}
	for _, s := range allowed {
		if !IsPublicIP(netip.MustParseAddr(s)) {
			t.Errorf("IsPublicIP(%s) = false, want true", s)
		}
	}
}

func TestReadAllLimited(t *testing.T) {
	if _, err := ReadAllLimited(strings.NewReader("hello"), 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := ReadAllLimited(strings.NewReader("hello!"), 5); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("want ErrBodyTooLarge, got %v", err)
	}
}

func TestCopyLimited(t *testing.T) {
	var sb strings.Builder
	if _, err := CopyLimited(&sb, strings.NewReader("abc"), 3); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := CopyLimited(&sb, strings.NewReader("abcd"), 3); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("want ErrBodyTooLarge, got %v", err)
	}
}

func TestRedirectLimit(t *testing.T) {
	client := NewClient(time.Second)
	via := make([]*http.Request, DefaultMaxRedirects)
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/next", nil)
	if err := client.CheckRedirect(req, via); !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("want ErrTooManyRedirects, got %v", err)
	}

	// 重定向到非 http(s) 协议必须被拒绝
	bad, _ := http.NewRequest(http.MethodGet, "ftp://example.com/", nil)
	if err := client.CheckRedirect(bad, via[:1]); !errors.Is(err, ErrSchemeNotAllowed) {
		t.Fatalf("want ErrSchemeNotAllowed, got %v", err)
	}
}

func TestDialBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := NewClient(2 * time.Second)
	resp, err := client.Get(srv.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("request to loopback must be blocked")
	}
	if !errors.Is(err, ErrAddressBlocked) {
		t.Fatalf("want ErrAddressBlocked, got %v", err)
	}
}
