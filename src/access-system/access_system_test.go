package accesssystem

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

// fakeProtector 模拟当前 Windows 用户的系统保护能力：base64 双编码模拟加密痕迹。
type fakeProtector struct {
	failProtect   bool
	failUnprotect bool
}

func (f fakeProtector) Protect(_ context.Context, plain string) (string, error) {
	if f.failProtect {
		return "", &protectFailureError{}
	}
	return base64.StdEncoding.EncodeToString([]byte(plain)), nil
}

func (f fakeProtector) Unprotect(_ context.Context, encrypted string) (string, error) {
	if f.failUnprotect {
		return "", &protectFailureError{}
	}
	payload, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

type protectFailureError struct{}

func (e *protectFailureError) Error() string { return "系统保护不可用" }

func newTestKeyManager(t *testing.T, dataDir string) *PersistentKeyManager {
	t.Helper()
	manager, err := NewPersistentKeyManager(dataDir)
	if err != nil {
		t.Fatalf("NewPersistentKeyManager() error = %v", err)
	}
	return manager.WithProtector(fakeProtector{})
}

func newTestPortManager(t *testing.T, dataDir string, keys *PersistentKeyManager) *PersistentPortManager {
	t.Helper()
	manager, err := NewPersistentPortManager(dataDir, keys)
	if err != nil {
		t.Fatalf("NewPersistentPortManager() error = %v", err)
	}
	return manager
}

func TestKeyManagerCreateAndRevealRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := newTestKeyManager(t, dir)

	created, err := manager.Create(ctx, "测试 Key", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.PlainKey == "" || created.ID == "" {
		t.Fatalf("Create() 返回缺少内容：%#v", created)
	}
	plain, err := manager.Reveal(ctx, created.ID)
	if err != nil {
		t.Fatalf("Reveal() error = %v", err)
	}
	if plain != created.PlainKey {
		t.Fatalf("Reveal() = %q, want %q", plain, created.PlainKey)
	}
}

func TestKeyManagerVerifyKeyMatchesEnabledUnexpired(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := newTestKeyManager(t, dir)
	created, err := manager.Create(ctx, "匹配 Key", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	result := manager.VerifyKey(ctx, created.PlainKey)
	if !result.Valid || result.KeyID != created.ID {
		t.Fatalf("VerifyKey() = %#v", result)
	}
	views, err := manager.List(ctx)
	if err != nil || len(views) != 1 {
		t.Fatalf("List() = %#v, err=%v", views, err)
	}
	if views[0].LastUsedAt == nil {
		t.Fatalf("VerifyKey() 未更新 lastUsedAt")
	}
}

func TestKeyManagerVerifyRejectsWrongKey(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := newTestKeyManager(t, dir)
	if _, err := manager.Create(ctx, "匹配 Key", nil); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result := manager.VerifyKey(ctx, "wrong-key"); result.Valid {
		t.Fatalf("VerifyKey() 错误 Key 通过验证：%#v", result)
	}
}

func TestKeyManagerVerifyRejectsDisabledKey(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := newTestKeyManager(t, dir)
	created, err := manager.Create(ctx, "停用 Key", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := manager.SetEnabled(ctx, created.ID, false); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	if result := manager.VerifyKey(ctx, created.PlainKey); result.Valid {
		t.Fatalf("VerifyKey() 停用 Key 通过验证：%#v", result)
	}
	if manager.HasValidKey() {
		t.Fatalf("HasValidKey() = true，期望没有有效 Key")
	}
}

func TestKeyManagerVerifyRejectsExpiredKey(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := newTestKeyManager(t, dir)
	expired := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	created, err := manager.Create(ctx, "过期 Key", &expired)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result := manager.VerifyKey(ctx, created.PlainKey); result.Valid {
		t.Fatalf("VerifyKey() 过期 Key 通过验证：%#v", result)
	}
	if manager.HasValidKey() {
		t.Fatalf("HasValidKey() = true，期望没有有效 Key")
	}
}

func TestKeyManagerHasValidKeyWithUnexpired(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := newTestKeyManager(t, dir)
	future := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	if _, err := manager.Create(ctx, "未来 Key", &future); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !manager.HasValidKey() {
		t.Fatalf("HasValidKey() = false，期望有有效 Key")
	}
}

func TestKeyManagerDeleteEndsConnections(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := newTestKeyManager(t, dir)
	created, err := manager.Create(ctx, "连接 Key", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	closer := &recordingCloser{}
	manager.RegisterConnection(created.ID, closer)
	if err := manager.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !closer.closed {
		t.Fatalf("Delete() 未结束持续连接")
	}
}

func TestKeyManagerDisableEndsConnections(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	manager := newTestKeyManager(t, dir)
	created, err := manager.Create(ctx, "连接 Key", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	closer := &recordingCloser{}
	manager.RegisterConnection(created.ID, closer)
	if err := manager.SetEnabled(ctx, created.ID, false); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	if !closer.closed {
		t.Fatalf("SetEnabled(false) 未结束持续连接")
	}
}

type recordingCloser struct {
	closed bool
}

func (c *recordingCloser) Close() error {
	c.closed = true
	return nil
}

func TestPortManagerAddAndList(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	keys := newTestKeyManager(t, dir)
	manager := newTestPortManager(t, dir, keys)
	created, err := manager.AddPort(ctx, "测试端口", 18080)
	if err != nil {
		t.Fatalf("AddPort() error = %v", err)
	}
	if created.DesiredState != "disabled" || created.ActualState != "stopped" {
		t.Fatalf("AddPort() 初始状态错误：%#v", created)
	}
	ports, err := manager.List(ctx)
	if err != nil || len(ports) != 1 || ports[0].Port != 18080 {
		t.Fatalf("List() = %#v, err=%v", ports, err)
	}
}

func TestPortManagerRejectsDuplicatePort(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	keys := newTestKeyManager(t, dir)
	manager := newTestPortManager(t, dir, keys)
	if _, err := manager.AddPort(ctx, "端口 A", 18080); err != nil {
		t.Fatalf("AddPort() error = %v", err)
	}
	if _, err := manager.AddPort(ctx, "端口 B", 18080); err == nil {
		t.Fatalf("AddPort() 重复端口未被拒绝")
	}
}

func TestPortManagerRejectsInvalidPort(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	keys := newTestKeyManager(t, dir)
	manager := newTestPortManager(t, dir, keys)
	for _, port := range []int{0, -1, 65536} {
		if _, err := manager.AddPort(ctx, "无效端口", port); err == nil {
			t.Fatalf("AddPort(%d) 未被拒绝", port)
		}
	}
}

func TestPortManagerEnableRequiresValidKey(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	keys := newTestKeyManager(t, dir)
	manager := newTestPortManager(t, dir, keys)
	port, err := manager.AddPort(ctx, "无 Key 端口", 18081)
	if err != nil {
		t.Fatalf("AddPort() error = %v", err)
	}
	if _, err := manager.EnablePort(ctx, port.ID); err == nil {
		t.Fatalf("EnablePort() 无有效 Key 时未被拒绝")
	}
}

func TestPortManagerDeleteStopsListening(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	keys := newTestKeyManager(t, dir)
	keys.WithProtector(fakeProtector{})
	if _, err := keys.Create(ctx, "有效 Key", nil); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	manager := newTestPortManager(t, dir, keys)
	port, err := manager.AddPort(ctx, "删除端口", 18082)
	if err != nil {
		t.Fatalf("AddPort() error = %v", err)
	}
	manager.SetLocalEntrypointPort(0)
	manager.SetHandler(nil)
	if _, err := manager.EnablePort(ctx, port.ID); err != nil {
		t.Fatalf("EnablePort() error = %v", err)
	}
	if err := manager.DeletePort(ctx, port.ID); err != nil {
		t.Fatalf("DeletePort() error = %v", err)
	}
	ports, err := manager.List(ctx)
	if err != nil || len(ports) != 0 {
		t.Fatalf("DeletePort() 后 List() = %#v, err=%v", ports, err)
	}
}

func TestMigrationConvertsLegacyBoxKey(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	metaDir := filepath.Join(dir, "meta")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacyKey := "legacy-fixed-key-value"
	if err := os.WriteFile(filepath.Join(metaDir, "box.key"), []byte(legacyKey+"\n"), 0o644); err != nil {
		t.Fatalf("write box.key: %v", err)
	}
	keys := newTestKeyManager(t, dir)
	ports := newTestPortManager(t, dir, keys)
	if err := migrateLegacyAccessConfig(ctx, dir, keys, ports); err != nil {
		t.Fatalf("migrateLegacyAccessConfig() error = %v", err)
	}
	keyViews, err := keys.List(ctx)
	if err != nil {
		t.Fatalf("List() keys error = %v", err)
	}
	if len(keyViews) != 1 || keyViews[0].Name != legacyConvertedKeyName {
		t.Fatalf("转换后 Key 记录 = %#v", keyViews)
	}
	plain, err := keys.Reveal(ctx, keyViews[0].ID)
	if err != nil || plain != legacyKey {
		t.Fatalf("转换后 Key 解密 = %q, err=%v", plain, err)
	}
	portList, err := ports.List(ctx)
	if err != nil {
		t.Fatalf("List() ports error = %v", err)
	}
	if len(portList) != 1 || portList[0].Port != legacyFixedPort || portList[0].DesiredState != types.PersistentPortDesiredEnabled {
		t.Fatalf("转换后端口记录 = %#v", portList)
	}
	if _, err := os.Stat(filepath.Join(metaDir, "box.key")); !os.IsNotExist(err) {
		t.Fatalf("旧 box.key 未删除")
	}
	if !isMigrationCompleted(dir) {
		t.Fatalf("转换完成标记未写入")
	}
}

func TestMigrationSkipsWithoutLegacyFile(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	keys := newTestKeyManager(t, dir)
	ports := newTestPortManager(t, dir, keys)
	if err := migrateLegacyAccessConfig(ctx, dir, keys, ports); err != nil {
		t.Fatalf("migrateLegacyAccessConfig() error = %v", err)
	}
	keyViews, _ := keys.List(ctx)
	if len(keyViews) != 0 {
		t.Fatalf("无旧文件时不应产生 Key 记录：%#v", keyViews)
	}
}

func TestStorageAtomicWriteKeepsOriginalOnFailure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	keys := newTestKeyManager(t, dir)
	created, err := keys.Create(ctx, "原有 Key", nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	// 构造一个无法解析的记录文件来验证读取验证
	payload := []byte("{ invalid json")
	path := persistentKeysPath(dir)
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	if _, err := readPersistentKeys(dir); err == nil {
		t.Fatalf("readPersistentKeys() 未拒绝损坏文件")
	}
	// 恢复原记录文件内容应保留原数据
	config := types.PersistentKeysConfig{Keys: []types.PersistentKey{{
		ID: created.ID, Name: created.Name, EncryptedKey: "base64", Enabled: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}}}
	if err := writePersistentKeys(dir, config); err != nil {
		t.Fatalf("writePersistentKeys() error = %v", err)
	}
	loaded, err := readPersistentKeys(dir)
	if err != nil || len(loaded.Keys) != 1 || loaded.Keys[0].ID != created.ID {
		t.Fatalf("重新读取 = %#v, err=%v", loaded, err)
	}
}
