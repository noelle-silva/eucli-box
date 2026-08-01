package main

import (
	"fmt"
	"strings"
	"time"
)

func resolveBuildTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().UTC(), nil
	}
	buildTime, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("build time must use RFC3339: %w", err)
	}
	return buildTime.UTC(), nil
}
