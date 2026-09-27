package requestrecord

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

const (
	redactedHeaderValue = "[REDACTED]"
	recordQueueSize     = 64
	recordWriteTimeout  = 30 * time.Second
)

var sensitiveRequestHeaders = map[string]struct{}{
	"authorization":       {},
	"x-api-key":           {},
	"proxy-authorization": {},
	"cookie":              {},
}

// System 是模型请求记录子系统：
// 作为网络系统装饰器注入模型供应商系统（Do/DoStream 转发并留痕），
// 同时为网关提供配置读写与记录查询。
type System interface {
	Do(ctx context.Context, req types.HTTPRequest) (types.HTTPResponse, error)
	DoStream(ctx context.Context, req types.HTTPRequest, onChunk types.HTTPStreamHandler) (types.HTTPResponse, error)

	LoadRequestRecordConfig(ctx context.Context) (types.RequestRecordConfig, error)
	SaveRequestRecordConfig(ctx context.Context, config types.RequestRecordConfig) (types.RequestRecordConfig, error)
	ListRequestRecords(ctx context.Context) ([]types.RequestRecordSummary, error)
	LoadRequestRecord(ctx context.Context, recordID string) (types.RequestRecord, error)
}

type NetworkSystem interface {
	Do(ctx context.Context, req types.HTTPRequest) (types.HTTPResponse, error)
	DoStream(ctx context.Context, req types.HTTPRequest, onChunk types.HTTPStreamHandler) (types.HTTPResponse, error)
}

type StorageSystem interface {
	LoadRequestRecordConfig(ctx context.Context) (types.RequestRecordConfig, error)
	SaveRequestRecordConfig(ctx context.Context, config types.RequestRecordConfig) (types.RequestRecordConfig, error)
	AppendRequestRecord(ctx context.Context, record types.RequestRecord) (types.RequestRecord, error)
	ListRequestRecords(ctx context.Context) ([]types.RequestRecordSummary, error)
	LoadRequestRecord(ctx context.Context, recordID string) (types.RequestRecord, error)
}

type system struct {
	network NetworkSystem
	storage StorageSystem
	queue   chan types.RequestRecord
}

func NewSystem(network NetworkSystem, storage StorageSystem) (System, error) {
	if network == nil {
		return nil, recordInvalid("network system dependency is required", nil)
	}
	if storage == nil {
		return nil, recordInvalid("storage system dependency is required", nil)
	}
	s := &system{network: network, storage: storage, queue: make(chan types.RequestRecord, recordQueueSize)}
	go s.runRecordWorker()
	return s, nil
}

// runRecordWorker 在后台顺序落盘记录；写盘失败不影响任何调用方。
func (s *system) runRecordWorker() {
	for record := range s.queue {
		ctx, cancel := context.WithTimeout(context.Background(), recordWriteTimeout)
		_, _ = s.storage.AppendRequestRecord(ctx, record)
		cancel()
	}
}

func (s *system) Do(ctx context.Context, req types.HTTPRequest) (types.HTTPResponse, error) {
	config, err := s.storage.LoadRequestRecordConfig(ctx)
	if err != nil || !config.Enabled {
		return s.network.Do(ctx, req)
	}
	response, doErr := s.network.Do(ctx, req)
	s.enqueueRecord(req, response, doErr)
	return response, doErr
}

func (s *system) DoStream(ctx context.Context, req types.HTTPRequest, onChunk types.HTTPStreamHandler) (types.HTTPResponse, error) {
	config, err := s.storage.LoadRequestRecordConfig(ctx)
	if err != nil || !config.Enabled {
		return s.network.DoStream(ctx, req, onChunk)
	}
	response, doErr := s.network.DoStream(ctx, req, onChunk)
	s.enqueueRecord(req, response, doErr)
	return response, doErr
}

func (s *system) LoadRequestRecordConfig(ctx context.Context) (types.RequestRecordConfig, error) {
	return s.storage.LoadRequestRecordConfig(ctx)
}

func (s *system) SaveRequestRecordConfig(ctx context.Context, config types.RequestRecordConfig) (types.RequestRecordConfig, error) {
	if err := validateRequestRecordConfig(config); err != nil {
		return types.RequestRecordConfig{}, err
	}
	return s.storage.SaveRequestRecordConfig(ctx, config)
}

func (s *system) ListRequestRecords(ctx context.Context) ([]types.RequestRecordSummary, error) {
	return s.storage.ListRequestRecords(ctx)
}

func (s *system) LoadRequestRecord(ctx context.Context, recordID string) (types.RequestRecord, error) {
	return s.storage.LoadRequestRecord(ctx, recordID)
}

// enqueueRecord 构建记录并放入后台写入队列；队列满时丢弃该条，绝不阻塞模型请求主链路。
func (s *system) enqueueRecord(req types.HTTPRequest, response types.HTTPResponse, doErr error) {
	record := types.RequestRecord{
		ID:              utils.NewID("request-record"),
		CreatedAt:       time.Now().UTC(),
		Method:          req.Method,
		URL:             req.URL,
		Headers:         redactRequestHeaders(req.Headers),
		Body:            string(req.Body),
		ResponseStatus:  response.StatusCode,
		ResponseHeaders: cloneResponseHeaders(response.Headers),
		ResponseBody:    string(response.Body),
		DurationMs:      response.Duration.Milliseconds(),
	}
	if doErr != nil {
		record.Error = doErr.Error()
	}
	select {
	case s.queue <- record:
	default:
		log.Printf("[request-record-system] record queue is full, dropping %s", record.ID)
	}
}

func redactRequestHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	redacted := make(map[string]string, len(headers))
	for name, value := range headers {
		if _, sensitive := sensitiveRequestHeaders[strings.ToLower(strings.TrimSpace(name))]; sensitive {
			redacted[name] = redactedHeaderValue
			continue
		}
		redacted[name] = value
	}
	return redacted
}

func cloneResponseHeaders(headers map[string][]string) map[string][]string {
	if len(headers) == 0 {
		return nil
	}
	cloned := make(map[string][]string, len(headers))
	for name, values := range headers {
		copied := make([]string, len(values))
		copy(copied, values)
		cloned[name] = copied
	}
	return cloned
}

func validateRequestRecordConfig(config types.RequestRecordConfig) error {
	limit := config.Limit
	if limit == 0 {
		limit = types.RequestRecordLimitDefault
	}
	if limit < types.RequestRecordLimitMin || limit > types.RequestRecordLimitMax {
		return recordInvalid(fmt.Sprintf("limit must be between %d and %d", types.RequestRecordLimitMin, types.RequestRecordLimitMax), nil)
	}
	return nil
}
