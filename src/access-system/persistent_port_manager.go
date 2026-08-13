package accesssystem

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

// keyValidityChecker 是端口管理器需要的长期 Key 有效性判断。
type keyValidityChecker interface {
	HasValidKey() bool
}

// PersistentPortManager 管理全部长期端口记录及其真实监听：
// 期望启用时尝试开放监听，维护实际状态和失败原因，业务端退出时全部关闭。
type PersistentPortManager struct {
	dataDir string
	keys    keyValidityChecker

	handlerMu sync.RWMutex
	handler   http.Handler

	entrypointMu sync.RWMutex
	entrypointPort int

	mu      sync.Mutex
	ports   []types.PersistentPort
	servers map[string]*http.Server
}

func NewPersistentPortManager(dataDir string, keys keyValidityChecker) (*PersistentPortManager, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, fmt.Errorf("访问设置数据目录不能为空")
	}
	manager := &PersistentPortManager{
		dataDir: dataDir,
		keys:    keys,
		servers: map[string]*http.Server{},
	}
	config, err := readPersistentPorts(dataDir)
	if err != nil {
		return nil, err
	}
	manager.ports = config.Ports
	return manager, nil
}

// SetHandler 注入长期端口统一鉴权和业务处理流程；每次端口收到连接时转发给该处理器。
func (m *PersistentPortManager) SetHandler(handler http.Handler) {
	m.handlerMu.Lock()
	m.handler = handler
	m.handlerMu.Unlock()
}

// SetLocalEntrypointPort 记录当前自动本机入口的真实端口，用于冲突检查。
func (m *PersistentPortManager) SetLocalEntrypointPort(port int) {
	m.entrypointMu.Lock()
	m.entrypointPort = port
	m.entrypointMu.Unlock()
}

// List 返回全部长期端口记录。
func (m *PersistentPortManager) List(ctx context.Context) ([]types.PersistentPort, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := make([]types.PersistentPort, 0, len(m.ports))
	result = append(result, m.ports...)
	return result, nil
}

// AddPort 创建新长期端口记录，默认停用；端口号必须有效且不与已有记录重复。
func (m *PersistentPortManager) AddPort(ctx context.Context, name string, port int) (types.PersistentPort, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return types.PersistentPort{}, fmt.Errorf("长期端口名称不能为空")
	}
	if port < 1 || port > 65535 {
		return types.PersistentPort{}, fmt.Errorf("长期端口号无效：%d", port)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return types.PersistentPort{}, err
	}
	if m.portExistsLocked(port) {
		return types.PersistentPort{}, fmt.Errorf("端口 %d 已被其他长期端口记录使用", port)
	}
	record := types.PersistentPort{
		ID:           utils.NewID("port"),
		Name:         name,
		Port:         port,
		DesiredState: types.PersistentPortDesiredDisabled,
		ActualState:  types.PersistentPortActualStopped,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	m.ports = append(m.ports, record)
	if err := m.saveLocked(ctx); err != nil {
		return types.PersistentPort{}, err
	}
	return record, nil
}

// SavePort 写入一条已经完整构造的端口记录，供旧配置转换使用。
func (m *PersistentPortManager) SavePort(ctx context.Context, record types.PersistentPort) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	for index := range m.ports {
		if m.ports[index].ID == record.ID {
			m.ports[index] = record
			return m.saveLocked(ctx)
		}
	}
	m.ports = append(m.ports, record)
	return m.saveLocked(ctx)
}

// EnablePort 启用长期端口：必须存在有效长期 Key；改期望状态后尝试开放监听。
func (m *PersistentPortManager) EnablePort(ctx context.Context, id string) (types.PersistentPort, error) {
	if m.keys == nil || !m.keys.HasValidKey() {
		return types.PersistentPort{}, fmt.Errorf("无法启用端口：当前没有有效的长期 Key")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return types.PersistentPort{}, err
	}
	index, ok := m.findPortLocked(id)
	if !ok {
		return types.PersistentPort{}, fmt.Errorf("长期端口不存在")
	}
	if m.entrypointConflictLocked(m.ports[index].Port) {
		return m.markFailedLocked(ctx, index, types.PersistentPortConflictLocalEntrypoint)
	}
	if m.duplicatePortLocked(m.ports[index].Port, index) {
		return m.markFailedLocked(ctx, index, types.PersistentPortConflictDuplicatePort)
	}
	m.ports[index].DesiredState = types.PersistentPortDesiredEnabled
	m.ports[index].ActualState = types.PersistentPortActualStopped
	m.ports[index].FailureReason = ""
	if err := m.saveLocked(ctx); err != nil {
		return types.PersistentPort{}, err
	}
	m.startPortLocked(ctx, index)
	return m.ports[index], nil
}

// DisablePort 停用长期端口：改期望状态并关闭监听。
func (m *PersistentPortManager) DisablePort(ctx context.Context, id string) (types.PersistentPort, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return types.PersistentPort{}, err
	}
	index, ok := m.findPortLocked(id)
	if !ok {
		return types.PersistentPort{}, fmt.Errorf("长期端口不存在")
	}
	m.ports[index].DesiredState = types.PersistentPortDesiredDisabled
	m.ports[index].ActualState = types.PersistentPortActualStopped
	m.ports[index].FailureReason = ""
	m.closePortLocked(index)
	if err := m.saveLocked(ctx); err != nil {
		return types.PersistentPort{}, err
	}
	return m.ports[index], nil
}

// DeletePort 删除长期端口记录：先停止监听再从记录中删除。
func (m *PersistentPortManager) DeletePort(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	index, ok := m.findPortLocked(id)
	if !ok {
		return fmt.Errorf("长期端口不存在")
	}
	m.closePortLocked(index)
	filtered := m.ports[:0]
	for _, port := range m.ports {
		if port.ID != id {
			filtered = append(filtered, port)
		}
	}
	m.ports = filtered
	return m.saveLocked(ctx)
}

// Start 启动全部期望启用的长期端口，供业务端启动后调用。
func (m *PersistentPortManager) Start(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return
	}
	for index := range m.ports {
		if m.ports[index].DesiredState == types.PersistentPortDesiredEnabled {
			m.startPortLocked(ctx, index)
		}
	}
}

// Shutdown 关闭所有长期端口监听，供业务端退出时调用。
func (m *PersistentPortManager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.servers {
		server := m.servers[id]
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_ = server.Shutdown(shutdownCtx)
		cancel()
		delete(m.servers, id)
	}
}

// saveLocked 持久化当前端口记录（调用方持有锁）。
func (m *PersistentPortManager) saveLocked(ctx context.Context) error {
	return writePersistentPorts(m.dataDir, types.PersistentPortsConfig{Ports: m.ports})
}

func (m *PersistentPortManager) findPortLocked(id string) (int, bool) {
	for index, port := range m.ports {
		if port.ID == id {
			return index, true
		}
	}
	return 0, false
}

func (m *PersistentPortManager) portExistsLocked(port int) bool {
	for _, record := range m.ports {
		if record.Port == port {
			return true
		}
	}
	return false
}

func (m *PersistentPortManager) duplicatePortLocked(port int, exceptIndex int) bool {
	for index, record := range m.ports {
		if index == exceptIndex {
			continue
		}
		if record.Port == port {
			return true
		}
	}
	return false
}

func (m *PersistentPortManager) entrypointConflictLocked(port int) bool {
	m.entrypointMu.RLock()
	defer m.entrypointMu.RUnlock()
	return m.entrypointPort > 0 && m.entrypointPort == port
}

func (m *PersistentPortManager) markFailedLocked(ctx context.Context, index int, reason string) (types.PersistentPort, error) {
	m.ports[index].DesiredState = types.PersistentPortDesiredEnabled
	m.ports[index].ActualState = types.PersistentPortActualFailed
	m.ports[index].FailureReason = reason
	if err := m.saveLocked(ctx); err != nil {
		return types.PersistentPort{}, err
	}
	return m.ports[index], nil
}

func (m *PersistentPortManager) startPortLocked(ctx context.Context, index int) {
	if _, running := m.servers[m.ports[index].ID]; running {
		return
	}
	handler := m.currentHandler()
	if handler == nil {
		m.ports[index].ActualState = types.PersistentPortActualFailed
		m.ports[index].FailureReason = "业务端访问处理流程未就绪"
		_ = m.saveLocked(ctx)
		return
	}
	listener, err := net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(m.ports[index].Port))
	if err != nil {
		m.ports[index].ActualState = types.PersistentPortActualFailed
		m.ports[index].FailureReason = realListenError(err)
		_ = m.saveLocked(ctx)
		return
	}
	server := &http.Server{Handler: handler}
	m.servers[m.ports[index].ID] = server
	m.ports[index].ActualState = types.PersistentPortActualRunning
	m.ports[index].FailureReason = ""
	_ = m.saveLocked(ctx)
	go func() {
		_ = server.Serve(listener)
	}()
}

func (m *PersistentPortManager) closePortLocked(index int) {
	server := m.servers[m.ports[index].ID]
	if server == nil {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = server.Shutdown(shutdownCtx)
	cancel()
	delete(m.servers, m.ports[index].ID)
}

func (m *PersistentPortManager) currentHandler() http.Handler {
	m.handlerMu.RLock()
	defer m.handlerMu.RUnlock()
	return m.handler
}

// realListenError 返回监听失败的真正原因；端口被占用时给出可读说明。
func realListenError(err error) string {
	var addressInUse *net.OpError
	if errors.As(err, &addressInUse) {
		return "端口已被占用"
	}
	return err.Error()
}
