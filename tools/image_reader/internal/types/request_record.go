package types

import "time"

const (
	RequestRecordLimitDefault = 100
	RequestRecordLimitMin     = 1
	RequestRecordLimitMax     = 1000
)

// RequestRecordConfig 是模型请求记录功能的全局配置。
type RequestRecordConfig struct {
	Enabled   bool      `json:"enabled"`
	Limit     int       `json:"limit"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// RequestRecord 是一条完整的模型请求记录：请求原文与响应原文。
type RequestRecord struct {
	ID              string              `json:"id"`
	CreatedAt       time.Time           `json:"createdAt"`
	Method          string              `json:"method"`
	URL             string              `json:"url"`
	Headers         map[string]string   `json:"headers,omitempty"`
	Body            string              `json:"body"`
	ResponseStatus  int                 `json:"responseStatus"`
	ResponseHeaders map[string][]string `json:"responseHeaders,omitempty"`
	ResponseBody    string              `json:"responseBody"`
	DurationMs      int64               `json:"durationMs"`
	Error           string              `json:"error,omitempty"`
}

// RequestRecordSummary 是请求记录列表的索引条目（仅摘要）。
type RequestRecordSummary struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"createdAt"`
	Method     string    `json:"method"`
	URL        string    `json:"url"`
	Status     int       `json:"status"`
	DurationMs int64     `json:"durationMs"`
	Error      string    `json:"error,omitempty"`
}
