package main

import "encoding/json"

const directProtocolVersion = 2

const directEventChatUpdated = "aiChat.chat.updated"

type requestFrame struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type responseFrame struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	OK     bool           `json:"ok"`
	Result any            `json:"result,omitempty"`
	Error  *responseError `json:"error,omitempty"`
}

type responseError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type eventFrame struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Payload any    `json:"payload,omitempty"`
}

type readyFrame struct {
	Type string   `json:"type"`
	IPC  readyIPC `json:"ipc"`
}

type readyIPC struct {
	Mode            string `json:"mode"`
	Transport       string `json:"transport"`
	URL             string `json:"url"`
	ProtocolVersion int    `json:"protocolVersion"`
}

func okResponse(id string, result any) responseFrame {
	return responseFrame{ID: id, Type: "response", OK: true, Result: result}
}

func errorResponseFor(id string, err error) responseFrame {
	code := "BACKEND_ERROR"
	message := "请求失败"
	if err != nil {
		message = err.Error()
		if coded, ok := err.(codedError); ok {
			code = coded.Code()
		}
		if detailed, ok := err.(detailedError); ok {
			return responseFrame{ID: id, Type: "response", OK: false, Error: &responseError{Code: code, Message: message, Details: detailed.Details()}}
		}
	}
	return responseFrame{ID: id, Type: "response", OK: false, Error: &responseError{Code: code, Message: message}}
}

type codedError interface {
	error
	Code() string
}

type detailedError interface {
	error
	Details() any
}

type appError struct {
	code    string
	message string
	details any
}

func (e appError) Error() string { return e.message }
func (e appError) Code() string  { return e.code }
func (e appError) Details() any  { return e.details }

func newError(code string, message string) error {
	return appError{code: code, message: message}
}

func newErrorWithDetails(code string, message string, details any) error {
	return appError{code: code, message: message, details: details}
}
