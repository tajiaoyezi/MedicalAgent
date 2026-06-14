package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ErrorClass string

const (
	ErrTimeout       ErrorClass = "timeout"
	ErrHTTP5xx       ErrorClass = "http_5xx"
	ErrRateLimit     ErrorClass = "rate_limit"
	ErrHealthDown    ErrorClass = "health_down"
	ErrMissingKey    ErrorClass = "missing_key"
	ErrNetworkBlock  ErrorClass = "network_blocked"
	ErrAuthError     ErrorClass = "auth_error"
	ErrInputTooLarge ErrorClass = "input_too_large"
	ErrContentSafety ErrorClass = "content_safety"
	ErrUnknown       ErrorClass = "unknown"
)

var fallbackableErrors = map[ErrorClass]bool{
	ErrTimeout: true, ErrHTTP5xx: true, ErrRateLimit: true,
	ErrHealthDown: true, ErrMissingKey: true, ErrNetworkBlock: true,
}

func IsFallbackable(cls ErrorClass) bool { return fallbackableErrors[cls] }

type ProviderError struct {
	Class ErrorClass
	Msg   string
}

func (e *ProviderError) Error() string { return e.Msg }

func newProviderError(cls ErrorClass, msg string) *ProviderError {
	return &ProviderError{Class: cls, Msg: msg}
}

type DeploymentKind string

const (
	DeployPublic  DeploymentKind = "public"
	DeployPrivate DeploymentKind = "private"
)

// NetworkPolicy: "allow_all" | "intranet_only" | "deny_egress" | ""（null）。
type ProviderConnection struct {
	ProviderID     string
	Kind           string // "model" | "visual"
	Name           string
	Protocol       string
	BackendKind    string
	DeploymentKind DeploymentKind
	BaseURL        string
	Credential     string // 已解密，仅用于外发
	Model          string
	TimeoutMs      int
	MaxRetries     int
	NetworkPolicy  string
}

// IsPrivateHost：仅内网/回环目标视为「私有」。比 Node 正则更严（医疗红线：私有/离线 provider 不得出公网）：
// IP 字面量按 CIDR 判定，显式拒绝 169.254/链路本地（云元数据 169.254.169.254）与 0.0.0.0；
// 主机名按锚定 DNS 后缀白名单（避免 10.evil.com / 127.0.0.1.evil.com 前缀绕过）。
func IsPrivateHost(hostname string) bool {
	h := strings.ToLower(strings.TrimSuffix(hostname, "."))
	if h == "localhost" || h == "host.docker.internal" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return false // 拒绝 169.254/fe80（云元数据）与 0.0.0.0/::
		}
		return ip.IsLoopback() || ip.IsPrivate() // 127/8、::1、RFC1918、fc00::/7
	}
	for _, suffix := range []string{".local", ".internal", ".svc"} { // .svc.cluster.local 由 .local 覆盖
		if strings.HasSuffix(h, suffix) {
			return true
		}
	}
	return false
}

// EnforceNetworkPolicy：私有化 provider 命中 deny_egress/intranet_only 时，公网域名目标被拦截。
func EnforceNetworkPolicy(conn ProviderConnection, u *url.URL) error {
	if conn.DeploymentKind != DeployPrivate {
		return nil
	}
	policy := conn.NetworkPolicy
	if policy == "" {
		policy = "intranet_only"
	}
	if policy == "allow_all" {
		return nil
	}
	if !IsPrivateHost(u.Hostname()) {
		return newProviderError(ErrNetworkBlock,
			fmt.Sprintf("私有化 provider「%s」network_policy=%s 禁止出网，但目标 %s 为公网域名，出网网关已拦截", conn.Name, policy, u.Hostname()))
	}
	return nil
}

func resolveURL(baseURL, path string) (*url.URL, error) {
	b := baseURL
	if !strings.HasSuffix(b, "/") {
		b += "/"
	}
	base, err := url.Parse(b)
	if err != nil {
		return nil, err
	}
	ref, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	return base.ResolveReference(ref), nil
}

// ProviderFetch：统一出站 POST，网络策略校验 + 超时 + 状态码→错误类别映射（不含重试）。返回响应 JSON 字节。
func ProviderFetch(conn ProviderConnection, path string, body any, headers map[string]string) ([]byte, error) {
	u, err := resolveURL(conn.BaseURL, path)
	if err != nil {
		return nil, newProviderError(ErrUnknown, fmt.Sprintf("provider「%s」base_url 非法：%s", conn.Name, conn.BaseURL))
	}
	if err := EnforceNetworkPolicy(conn, u); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(conn.TimeoutMs)*time.Millisecond)
	defer cancel()
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("content-type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// 重定向亦须再过网络策略（防 302 至公网/元数据绕过出网网关）；跨主机跳转剥离鉴权头。
	client := &http.Client{
		CheckRedirect: func(rreq *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return newProviderError(ErrHTTP5xx, "重定向过多")
			}
			if err := EnforceNetworkPolicy(conn, rreq.URL); err != nil {
				return err
			}
			if len(via) > 0 && rreq.URL.Host != via[0].URL.Host {
				rreq.Header.Del("Authorization")
				rreq.Header.Del("x-api-key")
			}
			return nil
		},
	}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, newProviderError(ErrTimeout, fmt.Sprintf("provider「%s」请求超时（%dms）", conn.Name, conn.TimeoutMs))
		}
		var pe *ProviderError
		if errors.As(err, &pe) { // 重定向触发的网络策略拦截，保留错误类别
			return nil, pe
		}
		return nil, newProviderError(ErrHTTP5xx, fmt.Sprintf("provider「%s」连接失败：%s", conn.Name, err.Error()))
	}
	defer res.Body.Close()
	rb, _ := io.ReadAll(res.Body)
	switch {
	case res.StatusCode == 401 || res.StatusCode == 403:
		return nil, newProviderError(ErrAuthError, fmt.Sprintf("provider「%s」鉴权失败（HTTP %d）", conn.Name, res.StatusCode))
	case res.StatusCode == 429:
		return nil, newProviderError(ErrRateLimit, fmt.Sprintf("provider「%s」被限流（HTTP 429）", conn.Name))
	case res.StatusCode >= 500:
		return nil, newProviderError(ErrHTTP5xx, fmt.Sprintf("provider「%s」服务端错误（HTTP %d）", conn.Name, res.StatusCode))
	case res.StatusCode >= 400:
		t := string(rb)
		if len(t) > 200 {
			t = t[:200]
		}
		return nil, newProviderError(ErrUnknown, fmt.Sprintf("provider「%s」请求错误（HTTP %d）%s", conn.Name, res.StatusCode, t))
	}
	return rb, nil
}

func authHeaders(conn ProviderConnection) map[string]string {
	if conn.Credential != "" {
		return map[string]string{"authorization": "Bearer " + conn.Credential}
	}
	return map[string]string{}
}
