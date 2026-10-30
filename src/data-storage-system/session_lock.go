package datastorage

import (
	"sync"
	"sync/atomic"
)

// scopeLock 是单个会话 scope 的锁。
//
//   - writers 串行化写者，覆盖「读快照 → 内存变更 → 编码 → 落名」整段，
//     写者之间的先后由它保证，绝不靠 mu 的长持有。
//   - mu 只负责「字节读取」与「文件替换（rename）」互斥：
//     读者只在 os.ReadFile 期间持读锁，写者只在 rename 期间持写锁，
//     编解码、归一化、fsync 全在锁外。
//
// 这把锁不覆盖 sessions/workspaces/<ws>/index.json：
// 该索引由同一工作区下所有角色共享，归属 catalog 锁。
type scopeLock struct {
	writers sync.Mutex
	mu      sync.RWMutex
}

// scopeLockRegistry 是叶子锁：只保护 map，绝不在持锁期间做 IO。
type scopeLockRegistry struct {
	mu    sync.Mutex
	locks map[string]*scopeLock
}

func (registry *scopeLockRegistry) get(key string) *scopeLock {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.locks == nil {
		registry.locks = make(map[string]*scopeLock)
	}
	lock, ok := registry.locks[key]
	if !ok {
		lock = &scopeLock{}
		registry.locks[key] = lock
	}
	return lock
}

func (s *system) scopeLock(scope sessionScope) (*scopeLock, error) {
	cleaned, err := cleanSessionScope(scope)
	if err != nil {
		return nil, err
	}
	return s.scopeLocks.get(cleaned.key()), nil
}

// sessionStorageMetrics 是会话存储的进程内计数：
// 只做原子累加，不参与任何业务判定，供测试与排障量化收益。
type sessionStorageMetrics struct {
	indexReads     atomic.Uint64
	indexBytes     atomic.Uint64
	indexWrites    atomic.Uint64
	indexRebuilds  atomic.Uint64
	indexRepairs   atomic.Uint64
	indexFailures  atomic.Uint64
	dataWrites     atomic.Uint64
	dataReads      atomic.Uint64
	catalogUpdates atomic.Uint64
}

// SessionStorageMetrics 是会话存储计数的只读快照。
type SessionStorageMetrics struct {
	IndexReads     uint64 `json:"indexReads"`
	IndexBytes     uint64 `json:"indexBytes"`
	IndexWrites    uint64 `json:"indexWrites"`
	IndexRebuilds  uint64 `json:"indexRebuilds"`
	IndexRepairs   uint64 `json:"indexRepairs"`
	IndexFailures  uint64 `json:"indexFailures"`
	DataWrites     uint64 `json:"dataWrites"`
	DataReads      uint64 `json:"dataReads"`
	CatalogUpdates uint64 `json:"catalogUpdates"`
}

func (s *system) SessionStorageMetrics() SessionStorageMetrics {
	return SessionStorageMetrics{
		IndexReads:     s.sessionMetrics.indexReads.Load(),
		IndexBytes:     s.sessionMetrics.indexBytes.Load(),
		IndexWrites:    s.sessionMetrics.indexWrites.Load(),
		IndexRebuilds:  s.sessionMetrics.indexRebuilds.Load(),
		IndexRepairs:   s.sessionMetrics.indexRepairs.Load(),
		IndexFailures:  s.sessionMetrics.indexFailures.Load(),
		DataWrites:     s.sessionMetrics.dataWrites.Load(),
		DataReads:      s.sessionMetrics.dataReads.Load(),
		CatalogUpdates: s.sessionMetrics.catalogUpdates.Load(),
	}
}
