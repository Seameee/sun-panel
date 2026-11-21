// Package safeurl 提供出站 HTTP 访问的统一安全策略：
//   - 只允许 http/https，禁止带凭据的 URL；
//   - 在拨号时（DNS 解析之后、建立连接之前）校验目标 IP，
//     同时覆盖重定向与 DNS rebinding；
//   - 限制重定向次数与响应体大小。
//
// 本包不依赖项目内其他包，便于单独测试。
package safeurl

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// DefaultTimeout 单次请求总超时（含 DNS、TLS、读 body）。
	DefaultTimeout = 10 * time.Second
	// DefaultMaxRedirects 允许的最大重定向次数。
	DefaultMaxRedirects = 5
)

var (
	ErrInvalidURL       = errors.New("safeurl: invalid url")
	ErrSchemeNotAllowed = errors.New("safeurl: only http and https are allowed")
	ErrCredentials      = errors.New("safeurl: url must not contain credentials")
	ErrAddressBlocked   = errors.New("safeurl: target address is not allowed")
	ErrTooManyRedirects = errors.New("safeurl: too many redirects")
	ErrBodyTooLarge     = errors.New("safeurl: response body exceeds limit")
)

// Parse 校验并返回 URL。仅接受 http/https，拒绝 userinfo 与非法端口。
func Parse(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\x00\r\n\t") {
		return nil, ErrInvalidURL
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, ErrSchemeNotAllowed
	}

	u.Scheme = strings.ToLower(u.Scheme)
	if u.User != nil {
		return nil, ErrCredentials
	}

	host := u.Hostname()
	if host == "" || strings.ContainsAny(host, "%\\") || strings.HasSuffix(u.Host, ":") {
		return nil, ErrInvalidURL
	}

	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, ErrInvalidURL
		}
	}

	return u, nil
}

// IsPublicIP 判断地址是否允许作为出站目标。
// 私网、回环、链路本地、组播、保留地址一律拒绝。
func IsPublicIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()

	if ip.IsUnspecified() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	if ip.IsPrivate() || (ip.Is6() && !ipv6Global.Contains(ip)) {
		return false
	}
	for _, prefix := range blocked {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

var (
	ipv6Global = netip.MustParsePrefix("2000::/3")
	blocked    = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("100.64.0.0/10"),   // CGNAT
		netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
		netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
		netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
		netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
		netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
		netip.MustParsePrefix("192.88.99.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"),   // reserved
		netip.MustParsePrefix("2001::/23"),     // 特殊用途和过渡协议
		netip.MustParsePrefix("2001:db8::/32"), // 文档地址
		netip.MustParsePrefix("2002::/16"),     // 6to4
		netip.MustParsePrefix("3fff::/20"),     // 文档地址
	}
)

// controlVet 在真正连接前校验解析后的地址，避免 DNS rebinding。
func controlVet(network, address string, _ syscall.RawConn) error {
	if network != "tcp4" && network != "tcp6" && network != "tcp" {
		return ErrAddressBlocked
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ErrAddressBlocked
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return ErrAddressBlocked
	}
	if !IsPublicIP(ip) {
		return ErrAddressBlocked
	}
	return nil
}

// NewClient 返回带安全策略的 HTTP 客户端。timeout<=0 时使用 DefaultTimeout。
func NewClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   controlVet,
	}

	transport := &http.Transport{
		// 不读取 HTTP_PROXY 等环境变量，避免代理绕过地址校验
		Proxy:                  nil,
		DialContext:            dialer.DialContext,
		ForceAttemptHTTP2:      false,
		MaxIdleConns:           4,
		MaxIdleConnsPerHost:    2,
		IdleConnTimeout:        30 * time.Second,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  5 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		ExpectContinueTimeout:  time.Second,
		// 不自动解压，避免压缩炸弹
		DisableCompression: true,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= DefaultMaxRedirects {
				return ErrTooManyRedirects
			}
			// 每一跳都重新校验；连接层仍会再校验解析后的 IP
			if _, err := Parse(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}

// NewRequest 创建 GET 请求，并在创建前校验 URL。
func NewRequest(method, rawURL string) (*http.Request, error) {
	parsed, err := Parse(rawURL)
	if err != nil {
		return nil, err
	}
	return http.NewRequest(method, parsed.String(), nil)
}

// Do 校验 URL 后发起请求。
func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, ErrInvalidURL
	}
	if _, err := Parse(req.URL.String()); err != nil {
		return nil, err
	}
	if client == nil {
		client = NewClient(0)
	}
	return client.Do(req)
}

// Get 是 Do 的便捷封装：校验 URL 后发起 GET 请求。
func Get(client *http.Client, rawURL string) (*http.Response, error) {
	req, err := NewRequest(http.MethodGet, rawURL)
	if err != nil {
		return nil, err
	}
	return Do(client, req)
}

// ReadAllLimited 读取至多 limit 字节；超出返回 ErrBodyTooLarge。
func ReadAllLimited(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, ErrBodyTooLarge
	}
	buf, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > limit {
		return nil, ErrBodyTooLarge
	}
	return buf, nil
}

// CopyLimited 复制至多 limit 字节，超出返回 ErrBodyTooLarge。
func CopyLimited(dst io.Writer, src io.Reader, limit int64) (int64, error) {
	if limit <= 0 {
		return 0, ErrBodyTooLarge
	}
	n, err := io.Copy(dst, io.LimitReader(src, limit+1))
	if err != nil {
		return n, err
	}
	if n > limit {
		return n, ErrBodyTooLarge
	}
	return n, nil
}
