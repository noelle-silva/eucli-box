package datastorage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

type requestRecordIndex struct {
	Records []types.RequestRecordSummary `json:"records"`
}

func (s *system) LoadRequestRecordConfig(ctx context.Context) (types.RequestRecordConfig, error) {
	return loadMetaConfig(ctx, s.paths.requestRecordConfigFile(), defaultRequestRecordConfig(), normalizeRequestRecordConfigForStorage)
}

func (s *system) SaveRequestRecordConfig(ctx context.Context, config types.RequestRecordConfig) (types.RequestRecordConfig, error) {
	config = normalizeRequestRecordConfigForStorage(config)
	config.UpdatedAt = time.Now().UTC()
	if err := writeJSON(ctx, s.paths.requestRecordConfigFile(), config); err != nil {
		return types.RequestRecordConfig{}, err
	}
	return config, nil
}

func defaultRequestRecordConfig() types.RequestRecordConfig {
	return normalizeRequestRecordConfigForStorage(types.RequestRecordConfig{})
}

func normalizeRequestRecordConfigForStorage(config types.RequestRecordConfig) types.RequestRecordConfig {
	if config.Limit == 0 {
		config.Limit = types.RequestRecordLimitDefault
	}
	if config.UpdatedAt.IsZero() {
		config.UpdatedAt = time.Now().UTC()
	}
	return config
}

func (s *system) AppendRequestRecord(ctx context.Context, record types.RequestRecord) (types.RequestRecord, error) {
	s.requestRecordMu.Lock()
	defer s.requestRecordMu.Unlock()
	if err := ctx.Err(); err != nil {
		return types.RequestRecord{}, storageWriteFailed("write cancelled", err)
	}
	if _, err := cleanID(record.ID); err != nil {
		return types.RequestRecord{}, err
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	dir, err := s.paths.safeJoin(s.paths.requestRecordsRoot(), record.ID)
	if err != nil {
		return types.RequestRecord{}, err
	}
	if err := writeJSON(ctx, filepath.Join(dir, "data.json"), record); err != nil {
		return types.RequestRecord{}, err
	}
	summary := requestRecordSummary(record)
	if err := writeJSON(ctx, filepath.Join(dir, "summary.json"), summary); err != nil {
		return types.RequestRecord{}, err
	}
	if err := s.updateRequestRecordIndex(ctx, summary); err != nil {
		return types.RequestRecord{}, err
	}
	return record, nil
}

func (s *system) ListRequestRecords(ctx context.Context) ([]types.RequestRecordSummary, error) {
	index, err := s.readRequestRecordIndex(ctx)
	if err != nil {
		return nil, err
	}
	return index.Records, nil
}

func (s *system) LoadRequestRecord(ctx context.Context, recordID string) (types.RequestRecord, error) {
	dir, err := s.paths.safeJoin(s.paths.requestRecordsRoot(), recordID)
	if err != nil {
		return types.RequestRecord{}, err
	}
	record, err := readJSON[types.RequestRecord](ctx, filepath.Join(dir, "data.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return types.RequestRecord{}, storageNotFound("request record was not found", err)
		}
		return types.RequestRecord{}, err
	}
	return record, nil
}

// updateRequestRecordIndex 把新摘要并入索引并执行环形裁剪，只读写小文件，不触碰任何记录正文。
func (s *system) updateRequestRecordIndex(ctx context.Context, summary types.RequestRecordSummary) error {
	config, err := s.LoadRequestRecordConfig(ctx)
	if err != nil {
		return err
	}
	index, err := s.readRequestRecordIndex(ctx)
	if err != nil {
		return err
	}
	records := make([]types.RequestRecordSummary, 0, len(index.Records)+1)
	for _, existing := range index.Records {
		if existing.ID == summary.ID {
			continue
		}
		records = append(records, existing)
	}
	records = append(records, summary)
	sortRequestRecordSummaries(records)
	records, err = s.trimRequestRecordIndex(ctx, records, config.Limit)
	if err != nil {
		return err
	}
	return writeIndex(ctx, s.requestRecordIndexFile(), requestRecordIndex{Records: records})
}

// rebuildRequestRecordIndex 依据磁盘事实重建索引：只读取每条记录的摘要，摘要缺失或损坏时从完整记录自愈补写。
func (s *system) rebuildRequestRecordIndex(ctx context.Context) error {
	config, err := s.LoadRequestRecordConfig(ctx)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(s.paths.requestRecordsRoot())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return storageReadFailed("failed to scan request records root", err)
	}
	records := make([]types.RequestRecordSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		summary, err := s.loadOrHealRequestRecordSummary(ctx, entry.Name())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		records = append(records, summary)
	}
	sortRequestRecordSummaries(records)
	records, err = s.trimRequestRecordIndex(ctx, records, config.Limit)
	if err != nil {
		return err
	}
	return writeIndex(ctx, s.requestRecordIndexFile(), requestRecordIndex{Records: records})
}

func (s *system) readRequestRecordIndex(ctx context.Context) (requestRecordIndex, error) {
	index := requestRecordIndex{Records: []types.RequestRecordSummary{}}
	stored, err := readJSON[requestRecordIndex](ctx, s.requestRecordIndexFile())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return index, nil
		}
		return requestRecordIndex{}, err
	}
	if stored.Records != nil {
		index.Records = stored.Records
	}
	return index, nil
}

// loadOrHealRequestRecordSummary 读取单条记录的摘要；摘要缺失或损坏时从完整记录重建并补写。
func (s *system) loadOrHealRequestRecordSummary(ctx context.Context, recordID string) (types.RequestRecordSummary, error) {
	dir, err := s.paths.safeJoin(s.paths.requestRecordsRoot(), recordID)
	if err != nil {
		return types.RequestRecordSummary{}, err
	}
	summaryPath := filepath.Join(dir, "summary.json")
	if dataFileExists(summaryPath) {
		if summary, err := readJSON[types.RequestRecordSummary](ctx, summaryPath); err == nil && strings.TrimSpace(summary.ID) != "" {
			return summary, nil
		}
	}
	record, err := readJSON[types.RequestRecord](ctx, filepath.Join(dir, "data.json"))
	if err != nil {
		return types.RequestRecordSummary{}, err
	}
	summary := requestRecordSummary(record)
	if err := writeJSON(ctx, summaryPath, summary); err != nil {
		return types.RequestRecordSummary{}, err
	}
	return summary, nil
}

// trimRequestRecordIndex 对索引执行环形裁剪：超出上限的最旧记录目录被删除，返回裁剪后的索引。
func (s *system) trimRequestRecordIndex(ctx context.Context, records []types.RequestRecordSummary, limit int) ([]types.RequestRecordSummary, error) {
	if limit < types.RequestRecordLimitMin {
		limit = types.RequestRecordLimitDefault
	}
	if len(records) <= limit {
		return records, nil
	}
	for _, stale := range records[limit:] {
		dir, err := s.paths.safeJoin(s.paths.requestRecordsRoot(), stale.ID)
		if err != nil {
			return nil, err
		}
		if err := os.RemoveAll(dir); err != nil {
			return nil, storageWriteFailed("failed to remove stale request record", err)
		}
	}
	return records[:limit], nil
}

func (s *system) requestRecordIndexFile() string {
	return filepath.Join(s.paths.requestRecordsRoot(), "index.json")
}

func sortRequestRecordSummaries(records []types.RequestRecordSummary) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ID > records[j].ID
		}
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
}

func requestRecordSummary(record types.RequestRecord) types.RequestRecordSummary {
	return types.RequestRecordSummary{ID: record.ID, CreatedAt: record.CreatedAt, Method: record.Method, URL: record.URL, Status: record.ResponseStatus, DurationMs: record.DurationMs, Error: record.Error}
}
