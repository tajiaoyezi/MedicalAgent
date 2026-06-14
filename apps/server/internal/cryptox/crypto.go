// Package cryptox 复刻 apps/api/src/services/model/crypto.ts 的 AES-256-GCM 凭据加解密与掩码。
// 格式：b64(iv).b64(tag).b64(ciphertext)，key=sha256(MODEL_CREDENTIAL_SECRET)。
// 关键：Go 的 gcm.Seal 把 16B tag 追加在密文末尾，须拆出单独编码以与 Node 的 getAuthTag 对齐。
package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

type Codec struct {
	key [32]byte
}

func New(secret string) *Codec {
	return &Codec{key: sha256.Sum256([]byte(secret))}
}

// Encrypt 返回 `b64(iv).b64(tag).b64(ciphertext)`；空输入返回 ""（调用方存 NULL）。
func (cd *Codec) Encrypt(plain string) string {
	if plain == "" {
		return ""
	}
	gcm := cd.gcm()
	if gcm == nil {
		return ""
	}
	iv := make([]byte, gcm.NonceSize()) // 12
	if _, err := rand.Read(iv); err != nil {
		return ""
	}
	sealed := gcm.Seal(nil, iv, []byte(plain), nil)
	tag := sealed[len(sealed)-16:]
	ct := sealed[:len(sealed)-16]
	b := base64.StdEncoding
	return strings.Join([]string{b.EncodeToString(iv), b.EncodeToString(tag), b.EncodeToString(ct)}, ".")
}

// Decrypt 解析 `iv.tag.ciphertext`；非法或空返回 ""（绝不抛错）。
func (cd *Codec) Decrypt(stored string) string {
	if stored == "" {
		return ""
	}
	parts := strings.Split(stored, ".")
	if len(parts) != 3 {
		return ""
	}
	b := base64.StdEncoding
	iv, e1 := b.DecodeString(parts[0])
	tag, e2 := b.DecodeString(parts[1])
	ct, e3 := b.DecodeString(parts[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return ""
	}
	gcm := cd.gcm()
	if gcm == nil {
		return ""
	}
	plain, err := gcm.Open(nil, iv, append(ct, tag...), nil)
	if err != nil {
		return ""
	}
	return string(plain)
}

// Mask 仅暴露是否已配置 + 末尾少量字符；未配置返回 nil（对齐 Node 的 null）。
func (cd *Codec) Mask(stored string) *string {
	if stored == "" {
		return nil
	}
	plain := cd.Decrypt(stored)
	var s string
	if plain == "" || len(plain) <= 6 {
		s = "****"
	} else {
		s = plain[:3] + "****" + plain[len(plain)-2:]
	}
	return &s
}

func (cd *Codec) gcm() cipher.AEAD {
	block, err := aes.NewCipher(cd.key[:])
	if err != nil {
		return nil
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil
	}
	return g
}
