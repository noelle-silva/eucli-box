package systemplugin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type pluginOrderFile struct {
	IDs []string `json:"ids"`
}

func (s *system) orderFilePath() string {
	return filepath.Join(s.sourceDir, "order.json")
}

func (s *system) readOrderIDs(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, pluginReadFailed("read cancelled", err)
	}
	payload, err := os.ReadFile(s.orderFilePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, pluginReadFailed("failed to read system plugin order", err)
	}
	var order pluginOrderFile
	if err := json.Unmarshal(payload, &order); err != nil {
		return nil, pluginReadFailed("failed to decode system plugin order", err)
	}
	return normalizeOrderIDs(order.IDs), nil
}

func (s *system) writeOrderIDs(ctx context.Context, ids []string) error {
	if err := ctx.Err(); err != nil {
		return pluginWriteFailed("write cancelled", err)
	}
	payload, err := json.MarshalIndent(pluginOrderFile{IDs: normalizeOrderIDs(ids)}, "", "  ")
	if err != nil {
		return pluginWriteFailed("failed to marshal system plugin order", err)
	}
	payload = append(payload, '\n')
	target := s.orderFilePath()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return pluginWriteFailed("failed to create system plugin order directory", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".tmp-*.json")
	if err != nil {
		return pluginWriteFailed("failed to create temporary system plugin order", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return pluginWriteFailed("failed to write temporary system plugin order", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return pluginWriteFailed("failed to sync temporary system plugin order", err)
	}
	if err := tmp.Close(); err != nil {
		return pluginWriteFailed("failed to close temporary system plugin order", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return pluginWriteFailed("failed to replace system plugin order", err)
	}
	return nil
}

func normalizeOrderIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func mergeOrderIDs(order []string, existing []string) []string {
	present := make(map[string]struct{}, len(existing))
	for _, id := range existing {
		present[id] = struct{}{}
	}
	result := make([]string, 0, len(existing))
	seen := make(map[string]struct{}, len(existing))
	for _, id := range normalizeOrderIDs(order) {
		if _, ok := present[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	remaining := make([]string, 0, len(existing)-len(result))
	for id := range present {
		if _, ok := seen[id]; ok {
			continue
		}
		remaining = append(remaining, id)
	}
	sort.Strings(remaining)
	return append(result, remaining...)
}
