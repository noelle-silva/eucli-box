package datastorage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
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
	if err := s.rebuildRequestRecordIndex(ctx); err != nil {
		return types.RequestRecord{}, err
	}
	return record, nil
}

func (s *system) ListRequestRecords(ctx context.Context) ([]types.RequestRecordSummary, error) {
	records, err := readObjects[types.RequestRecord](ctx, s.paths.requestRecordsRoot())
	if err != nil {
		return nil, err
	}
	sortRequestRecords(records)
	summaries := make([]types.RequestRecordSummary, 0, len(records))
	for _, record := range records {
		summaries = append(summaries, requestRecordSummary(record))
	}
	return summaries, nil
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

func (s *system) rebuildRequestRecordIndex(ctx context.Context) error {
	config, err := s.LoadRequestRecordConfig(ctx)
	if err != nil {
		return err
	}
	records, err := readObjects[types.RequestRecord](ctx, s.paths.requestRecordsRoot())
	if err != nil {
		return err
	}
	sortRequestRecords(records)
	limit := config.Limit
	if limit < types.RequestRecordLimitMin {
		limit = types.RequestRecordLimitDefault
	}
	if len(records) > limit {
		for _, stale := range records[limit:] {
			dir, err := s.paths.safeJoin(s.paths.requestRecordsRoot(), stale.ID)
			if err != nil {
				return err
			}
			if err := os.RemoveAll(dir); err != nil {
				return storageWriteFailed("failed to remove stale request record", err)
			}
		}
		records = records[:limit]
	}
	summaries := make([]types.RequestRecordSummary, 0, len(records))
	for _, record := range records {
		summaries = append(summaries, requestRecordSummary(record))
	}
	return writeIndex(ctx, filepath.Join(s.paths.requestRecordsRoot(), "index.json"), requestRecordIndex{Records: summaries})
}

func sortRequestRecords(records []types.RequestRecord) {
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
