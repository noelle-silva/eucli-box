package localrun

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	IdentityKindInstall = "install"
	IdentityKindData    = "data"
	IdentityKindRun     = "run"
	IdentityKindSession = "session"
	identityByteLength  = 32
	identityHexLength   = identityByteLength * 2
)

func NewIdentity(kind string) (string, error) {
	kind = strings.TrimSpace(kind)
	if !validIdentityKind(kind) {
		return "", fmt.Errorf("本机身份类型无效")
	}
	value := make([]byte, identityByteLength)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("生成本机身份失败：%w", err)
	}
	return kind + "-" + hex.EncodeToString(value), nil
}

func ValidateIdentity(value string, expectedKind string) error {
	value = strings.TrimSpace(value)
	expectedKind = strings.TrimSpace(expectedKind)
	if !validIdentityKind(expectedKind) {
		return fmt.Errorf("本机身份类型无效")
	}
	prefix := expectedKind + "-"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+identityHexLength {
		return fmt.Errorf("本机身份格式无效")
	}
	encoded := strings.TrimPrefix(value, prefix)
	if _, err := hex.DecodeString(encoded); err != nil || strings.ToLower(encoded) != encoded {
		return fmt.Errorf("本机身份格式无效")
	}
	return nil
}

func validIdentityKind(kind string) bool {
	switch kind {
	case IdentityKindInstall, IdentityKindData, IdentityKindRun, IdentityKindSession:
		return true
	default:
		return false
	}
}
