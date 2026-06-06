package everything

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type searchResult struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	FullPath   string `json:"fullPath"`
	Kind       string `json:"kind"`
	Size       string `json:"size"`
	ModifiedAt string `json:"modifiedAt"`
}

func parseSearchCSV(text string) ([]searchResult, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	if strings.TrimSpace(text) == "" {
		return []searchResult{}, nil
	}
	reader := csv.NewReader(strings.NewReader(text))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse Everything search output failed: %w", err)
	}
	results := make([]searchResult, 0, len(records))
	for _, record := range records {
		if len(record) == 1 && strings.TrimSpace(record[0]) == "" {
			continue
		}
		if len(record) < 4 {
			return nil, fmt.Errorf("Everything search output column mismatch")
		}
		name := record[0]
		path := record[1]
		fullPath := name
		if path != "" {
			fullPath = filepath.Join(path, name)
		}
		results = append(results, searchResult{Name: name, Path: path, FullPath: fullPath, Kind: resultKind(fullPath), Size: record[2], ModifiedAt: record[3]})
	}
	return results, nil
}

func resultKind(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "unknown"
	}
	if info.IsDir() {
		return "folder"
	}
	return "file"
}
