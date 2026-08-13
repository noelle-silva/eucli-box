package types

// PersistentPort 是长期端口记录：用户建立的对外访问入口，既接收本机也接收系统允许到达的外部连接。
type PersistentPort struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Port          int    `json:"port"`
	DesiredState  string `json:"desiredState"`
	ActualState   string `json:"actualState"`
	FailureReason string `json:"failureReason,omitempty"`
	CreatedAt     string `json:"createdAt"`
}

// PersistentPortsConfig 是长期端口记录文件容器。
type PersistentPortsConfig struct {
	Ports []PersistentPort `json:"ports"`
}

// PersistentKey 是长期 Key 记录：encryptedKey 使用 Windows DPAPI 按当前用户加密保存。
type PersistentKey struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	EncryptedKey string  `json:"encryptedKey"`
	Enabled      bool    `json:"enabled"`
	ExpiresAt    *string `json:"expiresAt"`
	CreatedAt    string  `json:"createdAt"`
	LastUsedAt   *string `json:"lastUsedAt"`
}

// PersistentKeysConfig 是长期 Key 记录文件容器。
type PersistentKeysConfig struct {
	Keys []PersistentKey `json:"keys"`
}

// PersistentKeyView 是返回给客户端的长期 Key 摘要，不包含加密内容。
type PersistentKeyView struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Enabled    bool    `json:"enabled"`
	ExpiresAt  *string `json:"expiresAt"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt *string `json:"lastUsedAt"`
}

// PersistentKeyCreated 是创建长期 Key 的一次性响应：明文完整 Key 只在此返回一次。
type PersistentKeyCreated struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PlainKey    string `json:"plainKey"`
	ExpiresAt   *string `json:"expiresAt"`
	CreatedAt   string `json:"createdAt"`
}

// PersistentPortState 是长期端口期望状态词表。
const (
	PersistentPortDesiredEnabled  = "enabled"
	PersistentPortDesiredDisabled = "disabled"
)

// PersistentPortActualState 是长期端口实际状态词表。
const (
	PersistentPortActualRunning = "running"
	PersistentPortActualStopped = "stopped"
	PersistentPortActualFailed  = "failed"
)

// PersistentPortConflictReason 是长期端口开放失败的原因。
const (
	PersistentPortConflictLocalEntrypoint = "与本机受托端口冲突"
	PersistentPortConflictDuplicatePort    = "端口已被其他长期端口记录使用"
)

// PersistentKeyVerifyResult 是长期 Key 核对结果。
type PersistentKeyVerifyResult struct {
	Valid bool   `json:"valid"`
	KeyID string `json:"keyId,omitempty"`
}
