package aiimage

import (
	"bytes"
	"mime/multipart"
	"net/textproto"
)

// multipartPart 是一个 multipart 表单分段：有 Filename 时是文件，否则是普通字段。
type multipartPart struct {
	Name        string
	Value       string
	Filename    string
	ContentType string
	Bytes       []byte
}

// buildMultipartFormData 组装 multipart 表单体并返回内容类型。
func buildMultipartFormData(parts []multipartPart) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		if part.Filename == "" {
			if err := writer.WriteField(part.Name, part.Value); err != nil {
				return nil, "", err
			}
			continue
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="`+part.Name+`"; filename="`+part.Filename+`"`)
		if part.ContentType != "" {
			header.Set("Content-Type", part.ContentType)
		}
		field, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", err
		}
		if _, err := field.Write(part.Bytes); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), writer.FormDataContentType(), nil
}
