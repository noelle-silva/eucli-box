package datastorage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

func TestRequestRecordConfigAndRingRotation(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()

	config, err := system.LoadRequestRecordConfig(ctx)
	if err != nil {
		t.Fatalf("LoadRequestRecordConfig(default) error = %v", err)
	}
	if config.Enabled || config.Limit != types.RequestRecordLimitDefault {
		t.Fatalf("default config = %#v", config)
	}

	if _, err := system.SaveRequestRecordConfig(ctx, types.RequestRecordConfig{Enabled: true, Limit: 3}); err != nil {
		t.Fatalf("SaveRequestRecordConfig() error = %v", err)
	}
	reloaded, err := system.LoadRequestRecordConfig(ctx)
	if err != nil {
		t.Fatalf("LoadRequestRecordConfig(saved) error = %v", err)
	}
	if !reloaded.Enabled || reloaded.Limit != 3 {
		t.Fatalf("saved config = %#v", reloaded)
	}
	assertFile(t, filepath.Join(system.paths.root, ".meta", "request-record.json"))

	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	ids := []string{"request-record-0", "request-record-1", "request-record-2", "request-record-3", "request-record-4"}
	for i, id := range ids {
		record := types.RequestRecord{
			ID:             id,
			CreatedAt:      now.Add(time.Duration(i) * time.Second),
			Method:         "POST",
			URL:            "https://api.example.com/v1/chat/completions",
			Body:           "request-body-" + id,
			ResponseStatus: 200,
			ResponseBody:   "response-body-" + id,
			DurationMs:     int64(100 + i),
		}
		if _, err := system.AppendRequestRecord(ctx, record); err != nil {
			t.Fatalf("AppendRequestRecord(%s) error = %v", id, err)
		}
	}

	records, err := system.ListRequestRecords(ctx)
	if err != nil {
		t.Fatalf("ListRequestRecords() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("records = %d want 3", len(records))
	}
	if records[0].ID != "request-record-4" || records[1].ID != "request-record-3" || records[2].ID != "request-record-2" {
		t.Fatalf("record order = %#v", records)
	}
	assertNoFile(t, filepath.Join(system.paths.root, "request-records", "request-record-0", "data.json"))
	assertNoFile(t, filepath.Join(system.paths.root, "request-records", "request-record-1", "data.json"))
	assertFile(t, filepath.Join(system.paths.root, "request-records", "index.json"))

	record, err := system.LoadRequestRecord(ctx, "request-record-4")
	if err != nil {
		t.Fatalf("LoadRequestRecord() error = %v", err)
	}
	if record.Body != "request-body-request-record-4" || record.ResponseBody != "response-body-request-record-4" || record.DurationMs != 104 {
		t.Fatalf("loaded record = %#v", record)
	}
}

func TestRequestRecordIndexRebuilds(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	if _, err := system.AppendRequestRecord(ctx, types.RequestRecord{ID: "request-record-1", CreatedAt: time.Now().UTC(), Method: "POST", URL: "https://api.example.com", Body: "{}"}); err != nil {
		t.Fatalf("AppendRequestRecord() error = %v", err)
	}
	indexPath := filepath.Join(system.paths.root, "request-records", "index.json")
	assertFile(t, indexPath)
	if err := os.Remove(indexPath); err != nil {
		t.Fatalf("Remove(index) error = %v", err)
	}
	if err := system.RebuildIndexes(ctx); err != nil {
		t.Fatalf("RebuildIndexes() error = %v", err)
	}
	assertFile(t, indexPath)
}
