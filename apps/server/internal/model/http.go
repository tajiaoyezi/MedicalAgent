package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
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

var privateHostRE = regexp.MustCompile(`(?i)^(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|169\.254\.|host\.docker\.internal$|.*\.local$|.*\.internal$|.*\.svc$|.*\.svc\.cluster\.local$)`)

func IsPrivateHost(hostname string) bool {
	h := strings.ToLower(hostname)
	if h == "host.docker.internal" {
		return true
	}
	return privateHostRE.MatchString(h)
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
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, newProviderError(ErrTimeout, fmt.Sprintf("provider「%s」请求超时（%dms）", conn.Name, conn.TimeoutMs))
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
