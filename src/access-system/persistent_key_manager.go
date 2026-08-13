package accesssystem

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

// secretProtector 抽象当前 Windows 用户的系统保护能力，便于测试注入。
type secretProtector interface {
	Protect(ctx context.Context, plainSecret string) (string, error)
	Unprotect(ctx context.Context, encryptedSecret string) (string, error)
}

type keyRecordUpdate func(record *types.PersistentKey)

// PersistentKeyManager 管理全部长期 Key 记录：创建、启停、有效期、查看、核对和删除。
// 记录只保存 DPAPI 加密内容，完整 Key 不进入日志和界面以外的位置。
type PersistentKeyManager struct {
	dataDir string
	protect secretProtector
	now     func() time.Time

	mu          sync.Mutex
	keys        []types.PersistentKey
	connections map[string]map[io.Closer]struct{}
}

func NewPersistentKeyManager(dataDir string) (*PersistentKeyManager, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, fmt.Errorf("访问设置数据目录不能为空")
	}
	manager := &PersistentKeyManager{
		dataDir:     dataDir,
		protect:     dpapiProtector{},
		now:         func() time.Time { return time.Now().UTC() },
		connections: map[string]map[io.Closer]struct{}{},
	}
	config, err := readPersistentKeys(dataDir)
	if err != nil {
		return nil, err
	}
	manager.keys = config.Keys
	return manager, nil
}

// WithProtector 注入系统保护实现，主要用于测试。
func (m *PersistentKeyManager) WithProtector(protector secretProtector) *PersistentKeyManager {
	if protector != nil {
		m.protect = protector
	}
	return m
}

// List 返回全部长期 Key 摘要，不包含加密内容。
func (m *PersistentKeyManager) List(ctx context.Context) ([]types.PersistentKeyView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	views := make([]types.PersistentKeyView, 0, len(m.keys))
	for _, key := range m.keys {
		views = append(views, keyView(key))
	}
	return views, nil
}

// Create 生成随机完整 Key，使用系统保护加密后保存；默认启用；返回明文完整 Key（仅此一次）。
func (m *PersistentKeyManager) Create(ctx context.Context, name string, expiresAt *string) (types.PersistentKeyCreated, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return types.PersistentKeyCreated{}, fmt.Errorf("长期 Key 名称不能为空")
	}
	if err := validateExpiresAt(expiresAt); err != nil {
		return types.PersistentKeyCreated{}, err
	}
	plainKey, err := newRandomKey()
	if err != nil {
		return types.PersistentKeyCreated{}, fmt.Errorf("生成长期 Key 失败：%w", err)
	}
	encrypted, err := m.protect.Protect(ctx, plainKey)
	if err != nil {
		return types.PersistentKeyCreated{}, fmt.Errorf("保护长期 Key 失败：%w", err)
	}
	now := m.now()
	record := types.PersistentKey{
		ID:           utils.NewID("key"),
		Name:         name,
		EncryptedKey: encrypted,
		Enabled:      true,
		ExpiresAt:    normalizeExpiresAt(expiresAt),
		CreatedAt:    now.Format(time.RFC3339Nano),
		LastUsedAt:   nil,
	}
	if err := m.updateLocked(ctx, nil, func(records *[]types.PersistentKey) {
		*records = append(*records, record)
	}); err != nil {
		return types.PersistentKeyCreated{}, err
	}
	return types.PersistentKeyCreated{ID: record.ID, Name: record.Name, PlainKey: plainKey, ExpiresAt: record.ExpiresAt, CreatedAt: record.CreatedAt}, nil
}

// Reveal 解密指定长期 Key 并返回完整明文；找不到记录时明确失败。
func (m *PersistentKeyManager) Reveal(ctx context.Context, id string) (string, error) {
	m.mu.Lock()
	key, ok := findKey(m.keys, id)
	m.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("长期 Key 不存在")
	}
	plain, err := m.protect.Unprotect(ctx, key.EncryptedKey)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(plain) == "" {
		return "", fmt.Errorf("长期 Key 解密结果为空")
	}
	return plain, nil
}

// SaveKey 保存一条已经给出明文的 Key 记录（旧配置转换专用）：
// 使用系统保护加密明文后保存，转换失败时保留原调用方资料。
func (m *PersistentKeyManager) SaveKey(ctx context.Context, name string, plainKey string, enabled bool, expiresAt *string) (types.PersistentKeyCreated, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return types.PersistentKeyCreated{}, fmt.Errorf("长期 Key 名称不能为空")
	}
	if strings.TrimSpace(plainKey) == "" {
		return types.PersistentKeyCreated{}, fmt.Errorf("待保存的长期 Key 不能为空")
	}
	if err := validateExpiresAt(expiresAt); err != nil {
		return types.PersistentKeyCreated{}, err
	}
	encrypted, err := m.protect.Protect(ctx, plainKey)
	if err != nil {
		return types.PersistentKeyCreated{}, fmt.Errorf("保护长期 Key 失败：%w", err)
	}
	now := m.now()
	record := types.PersistentKey{
		ID:           utils.NewID("key"),
		Name:         name,
		EncryptedKey: encrypted,
		Enabled:      enabled,
		ExpiresAt:    normalizeExpiresAt(expiresAt),
		CreatedAt:    now.Format(time.RFC3339Nano),
		LastUsedAt:   nil,
	}
	if err := m.updateLocked(ctx, nil, func(records *[]types.PersistentKey) {
		*records = append(*records, record)
	}); err != nil {
		return types.PersistentKeyCreated{}, err
	}
	return types.PersistentKeyCreated{ID: record.ID, Name: record.Name, PlainKey: plainKey, ExpiresAt: record.ExpiresAt, CreatedAt: record.CreatedAt}, nil
}

// SetEnabled 修改长期 Key 启用状态；停用时立即结束使用该 Key 的所有持续连接。
func (m *PersistentKeyManager) SetEnabled(ctx context.Context, id string, enabled bool) error {
	return m.updateLocked(ctx, &id, func(records *[]types.PersistentKey) {
		for index := range *records {
			if (*records)[index].ID == id {
				(*records)[index].Enabled = enabled
			}
		}
	})
}

// SetExpiration 修改长期 Key 有效期；null 表示永不过期。
func (m *PersistentKeyManager) SetExpiration(ctx context.Context, id string, expiresAt *string) error {
	if err := validateExpiresAt(expiresAt); err != nil {
		return err
	}
	normalized := normalizeExpiresAt(expiresAt)
	return m.updateLocked(ctx, &id, func(records *[]types.PersistentKey) {
		for index := range *records {
			if (*records)[index].ID == id {
				(*records)[index].ExpiresAt = normalized
			}
		}
	})
}

// Delete 删除长期 Key 记录并立即结束使用该 Key 的所有持续连接。
func (m *PersistentKeyManager) Delete(ctx context.Context, id string) error {
	return m.updateLocked(ctx, &id, func(records *[]types.PersistentKey) {
		filtered := (*records)[:0]
		for _, key := range *records {
			if key.ID != id {
				filtered = append(filtered, key)
			}
		}
		*records = filtered
	})
}

// HasValidKey 判断是否存在至少一个启用且未过期的长期 Key。
func (m *PersistentKeyManager) HasValidKey() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range m.keys {
		if key.Enabled && !keyExpired(key, m.now()) {
			return true
		}
	}
	return false
}

// VerifyKey 核对提供的 Key 与全部启用且未过期长期 Key 的匹配情况。
// 匹配成功时更新该 Key 的最后使用时间；任何错误都返回验证失败。
func (m *PersistentKeyManager) VerifyKey(ctx context.Context, providedKey string) types.PersistentKeyVerifyResult {
	providedKey = strings.TrimSpace(providedKey)
	if providedKey == "" {
		return types.PersistentKeyVerifyResult{Valid: false}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for index := range m.keys {
		key := &m.keys[index]
		if !key.Enabled {
			continue
		}
		if keyExpired(*key, now) {
			m.terminateConnectionsLocked(key.ID)
			continue
		}
		plain, err := m.protect.Unprotect(ctx, key.EncryptedKey)
		if err != nil {
			continue
		}
		if constantTimeEqual(providedKey, plain) {
			updated := now.Format(time.RFC3339Nano)
			key.LastUsedAt = &updated
			config := types.PersistentKeysConfig{Keys: m.keys}
			if err := writePersistentKeys(m.dataDir, config); err != nil {
				log.Printf("access-system: 更新长期 Key 最后使用时间失败（不影响本次验证）：%v", err)
			}
			return types.PersistentKeyVerifyResult{Valid: true, KeyID: key.ID}
		}
	}
	return types.PersistentKeyVerifyResult{Valid: false}
}

// RegisterConnection 登记一条使用指定长期 Key 建立的持续连接。
func (m *PersistentKeyManager) RegisterConnection(keyID string, closer io.Closer) {
	if closer == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.keyExistsLocked(keyID) {
		_ = closer.Close()
		return
	}
	connections := m.connections[keyID]
	if connections == nil {
		connections = map[io.Closer]struct{}{}
		m.connections[keyID] = connections
	}
	connections[closer] = struct{}{}
}

// UnregisterConnection 注销一条持续连接。
func (m *PersistentKeyManager) UnregisterConnection(keyID string, closer io.Closer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if connections := m.connections[keyID]; connections != nil {
		delete(connections, closer)
		if len(connections) == 0 {
			delete(m.connections, keyID)
		}
	}
}

// CloseAllConnections 结束全部持续连接，用于业务端退出。
func (m *PersistentKeyManager) CloseAllConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for keyID := range m.connections {
		m.terminateConnectionsLocked(keyID)
	}
}

func (m *PersistentKeyManager) updateLocked(ctx context.Context, targetID *string, mutate func(*[]types.PersistentKey)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	mutate(&m.keys)
	config := types.PersistentKeysConfig{Keys: m.keys}
	if err := writePersistentKeys(m.dataDir, config); err != nil {
		return err
	}
	if targetID != nil {
		m.terminateConnectionsLocked(*targetID)
	}
	return nil
}

func (m *PersistentKeyManager) keyExistsLocked(id string) bool {
	_, ok := findKey(m.keys, id)
	return ok
}

func (m *PersistentKeyManager) terminateConnectionsLocked(keyID string) {
	connections := m.connections[keyID]
	if len(connections) == 0 {
		delete(m.connections, keyID)
		return
	}
	for closer := range connections {
		_ = closer.Close()
	}
	delete(m.connections, keyID)
}

func findKey(keys []types.PersistentKey, id string) (types.PersistentKey, bool) {
	for _, key := range keys {
		if key.ID == id {
			return key, true
		}
	}
	return types.PersistentKey{}, false
}

func keyView(key types.PersistentKey) types.PersistentKeyView {
	return types.PersistentKeyView{
		ID:         key.ID,
		Name:       key.Name,
		Enabled:    key.Enabled,
		ExpiresAt:  key.ExpiresAt,
		CreatedAt:  key.CreatedAt,
		LastUsedAt: key.LastUsedAt,
	}
}

func keyExpired(key types.PersistentKey, now time.Time) bool {
	if key.ExpiresAt == nil {
		return false
	}
	expires, err := time.Parse(time.RFC3339Nano, *key.ExpiresAt)
	if err != nil {
		return true
	}
	return !expires.After(now)
}

// normalizeExpiresAt 校验并归一化有效期；null 表示永不过期。
func validateExpiresAt(expiresAt *string) error {
	if expiresAt == nil {
		return nil
	}
	value := strings.TrimSpace(*expiresAt)
	if value == "" {
		return fmt.Errorf("长期 Key 有效期不能为空字符串")
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fmt.Errorf("长期 Key 有效期格式无效")
	}
	if parsed.Equal(time.Time{}) {
		return fmt.Errorf("长期 Key 有效期格式无效")
	}
	return nil
}

func normalizeExpiresAt(expiresAt *string) *string {
	if expiresAt == nil {
		return nil
	}
	value := strings.TrimSpace(*expiresAt)
	if value == "" {
		return nil
	}
	return &value
}

// newRandomKey 使用 crypto/rand 生成 32 字节随机内容并转为 base64 作为完整长期 Key。
func newRandomKey() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(value), nil
}

func constantTimeEqual(left string, right string) bool {
	leftBytes := []byte(left)
	rightBytes := []byte(right)
	if len(leftBytes) != len(rightBytes) {
		return false
	}
	return subtle.ConstantTimeCompare(leftBytes, rightBytes) == 1
}

// dpapiProtector 使用 Windows DPAPI 系统保护实现 secretProtector。
type dpapiProtector struct{}

func (dpapiProtector) Protect(ctx context.Context, plainSecret string) (string, error) {
	return protectSecret(ctx, plainSecret)
}

func (dpapiProtector) Unprotect(ctx context.Context, encryptedSecret string) (string, error) {
	return unprotectSecret(ctx, encryptedSecret)
}
