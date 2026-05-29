package utils

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

var idCounter uint64

func NewID(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "id"
	}
	count := atomic.AddUint64(&idCounter, 1)
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UTC().UnixNano(), count)
}
