package datastorage

import (
	"context"
	"fmt"
	"path/filepath"

	"eucli-box/pkg/types"
)

func (s *system) SaveCallRecord(ctx context.Context, record types.CallRecord) error {
	return writeJSON(ctx, filepath.Join(s.paths.metaRoot(), fmt.Sprintf("calls-%s.json", record.ID)), record)
}
