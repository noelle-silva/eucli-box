package accesssystem

import (
	"context"
	"net/http"

	"eucli-box/pkg/types"
)

// System 是业务端长期访问能力的统一入口：
// 长期端口负责接收连接，长期 Key 负责访问身份，两者共同服务于本机与系统允许到达的外部访问。
type System interface {
	// 长期端口
	ListPorts(ctx context.Context) ([]types.PersistentPort, error)
	AddPort(ctx context.Context, name string, port int) (types.PersistentPort, error)
	EnablePort(ctx context.Context, id string) (types.PersistentPort, error)
	DisablePort(ctx context.Context, id string) (types.PersistentPort, error)
	DeletePort(ctx context.Context, id string) error

	// 长期 Key
	ListKeys(ctx context.Context) ([]types.PersistentKeyView, error)
	CreateKey(ctx context.Context, name string, expiresAt *string) (types.PersistentKeyCreated, error)
	RevealKey(ctx context.Context, id string) (string, error)
	SetKeyEnabled(ctx context.Context, id string, enabled bool) error
	SetKeyExpiration(ctx context.Context, id string, expiresAt *string) error
	DeleteKey(ctx context.Context, id string) error

	// 运行
	VerifyKey(ctx context.Context, providedKey string) types.PersistentKeyVerifyResult
	SetHandler(handler http.Handler)
	SetLocalEntrypointPort(port int)
	Start(ctx context.Context)
	Shutdown(ctx context.Context)
	RegisterConnection(keyID string, closer interface{ Close() error })
	UnregisterConnection(keyID string, closer interface{ Close() error })
}

type accessSystem struct {
	keys  *PersistentKeyManager
	ports *PersistentPortManager
}

// NewSystem 组装长期访问系统。dataDir 是业务端数据目录，用于持久化访问设置记录。
func NewSystem(dataDir string) (System, error) {
	keys, err := NewPersistentKeyManager(dataDir)
	if err != nil {
		return nil, err
	}
	ports, err := NewPersistentPortManager(dataDir, keys)
	if err != nil {
		return nil, err
	}
	return &accessSystem{keys: keys, ports: ports}, nil
}

// NewSystemWithProtector 注入测试用的系统保护实现。
func NewSystemWithProtector(dataDir string, protector secretProtector) (System, error) {
	keys, err := NewPersistentKeyManager(dataDir)
	if err != nil {
		return nil, err
	}
	keys.WithProtector(protector)
	ports, err := NewPersistentPortManager(dataDir, keys)
	if err != nil {
		return nil, err
	}
	return &accessSystem{keys: keys, ports: ports}, nil
}

// MigrateLegacyConfig 执行旧固定设置一次性转换。
func MigrateLegacyConfig(ctx context.Context, system System, dataDir string) error {
	internal, ok := system.(*accessSystem)
	if !ok {
		return &migrationUnsupportedError{}
	}
	return migrateLegacyAccessConfig(ctx, dataDir, internal.keys, internal.ports)
}

type migrationUnsupportedError struct{}

func (e *migrationUnsupportedError) Error() string {
	return "访问系统不支持旧配置转换"
}

func (s *accessSystem) ListPorts(ctx context.Context) ([]types.PersistentPort, error) {
	return s.ports.List(ctx)
}

func (s *accessSystem) AddPort(ctx context.Context, name string, port int) (types.PersistentPort, error) {
	return s.ports.AddPort(ctx, name, port)
}

func (s *accessSystem) EnablePort(ctx context.Context, id string) (types.PersistentPort, error) {
	return s.ports.EnablePort(ctx, id)
}

func (s *accessSystem) DisablePort(ctx context.Context, id string) (types.PersistentPort, error) {
	return s.ports.DisablePort(ctx, id)
}

func (s *accessSystem) DeletePort(ctx context.Context, id string) error {
	return s.ports.DeletePort(ctx, id)
}

func (s *accessSystem) ListKeys(ctx context.Context) ([]types.PersistentKeyView, error) {
	return s.keys.List(ctx)
}

func (s *accessSystem) CreateKey(ctx context.Context, name string, expiresAt *string) (types.PersistentKeyCreated, error) {
	return s.keys.Create(ctx, name, expiresAt)
}

func (s *accessSystem) RevealKey(ctx context.Context, id string) (string, error) {
	return s.keys.Reveal(ctx, id)
}

func (s *accessSystem) SetKeyEnabled(ctx context.Context, id string, enabled bool) error {
	return s.keys.SetEnabled(ctx, id, enabled)
}

func (s *accessSystem) SetKeyExpiration(ctx context.Context, id string, expiresAt *string) error {
	return s.keys.SetExpiration(ctx, id, expiresAt)
}

func (s *accessSystem) DeleteKey(ctx context.Context, id string) error {
	return s.keys.Delete(ctx, id)
}

func (s *accessSystem) VerifyKey(ctx context.Context, providedKey string) types.PersistentKeyVerifyResult {
	return s.keys.VerifyKey(ctx, providedKey)
}

func (s *accessSystem) SetHandler(handler http.Handler) {
	s.ports.SetHandler(handler)
}

func (s *accessSystem) SetLocalEntrypointPort(port int) {
	s.ports.SetLocalEntrypointPort(port)
}

func (s *accessSystem) Start(ctx context.Context) {
	s.ports.Start(ctx)
}

func (s *accessSystem) Shutdown(ctx context.Context) {
	s.ports.Shutdown(ctx)
	s.keys.CloseAllConnections()
}

func (s *accessSystem) RegisterConnection(keyID string, closer interface{ Close() error }) {
	s.keys.RegisterConnection(keyID, closer)
}

func (s *accessSystem) UnregisterConnection(keyID string, closer interface{ Close() error }) {
	s.keys.UnregisterConnection(keyID, closer)
}
