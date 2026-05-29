package types

import "time"

type HTTPBodyKind string

const (
	HTTPBodyNone  HTTPBodyKind = "none"
	HTTPBodyJSON  HTTPBodyKind = "json"
	HTTPBodyForm  HTTPBodyKind = "form"
	HTTPBodyText  HTTPBodyKind = "text"
	HTTPBodyBytes HTTPBodyKind = "bytes"
)

type HTTPRequest struct {
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers,omitempty"`
	BodyKind HTTPBodyKind      `json:"bodyKind"`
	Body     []byte            `json:"body,omitempty"`
	Timeout  time.Duration     `json:"timeout"`
}

type HTTPResponse struct {
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Body       []byte              `json:"body,omitempty"`
	Duration   time.Duration       `json:"duration"`
}
