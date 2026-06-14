package editor

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWT 复刻 services/onlyoffice-jwt.ts：HS256 + 1h 过期；禁用时签名返回 ""、验签返回空 claims。
type JWT struct {
	secret  []byte
	enabled bool
}

func NewJWT(secret string, enabled bool) *JWT {
	return &JWT{secret: []byte(secret), enabled: enabled}
}

func (j *JWT) Sign(payload map[string]any) string {
	if !j.enabled {
		return ""
	}
	claims := jwt.MapClaims{}
	for k, v := range payload {
		claims[k] = v
	}
	claims["exp"] = time.Now().Add(time.Hour).Unix()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(j.secret)
	if err != nil {
		return ""
	}
	return s
}

// Verify 验签并返回 claims（含 exp/iat）；禁用时返回空 map+true（对齐 Node 的 {}）。
func (j *JWT) Verify(tokenStr string) (map[string]any, bool) {
	if !j.enabled {
		return map[string]any{}, true
	}
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return j.secret, nil
	})
	if err != nil || !tok.Valid {
		return nil, false
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return nil, false
	}
	return map[string]any(claims), true
}

// Wrap 复刻 wrapOnlyofficeConfig：启用时在顶层附 token，否则原样返回。
func (j *JWT) Wrap(editorConfig map[string]any) map[string]any {
	if !j.enabled {
		return editorConfig
	}
	out := map[string]any{}
	for k, v := range editorConfig {
		out[k] = v
	}
	out["token"] = j.Sign(editorConfig)
	return out
}

func (j *JWT) Enabled() bool { return j.enabled }
