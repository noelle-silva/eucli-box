package datastorage

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

var imageDataURLPattern = regexp.MustCompile(`^data:(image/(png|jpeg|jpg|webp|gif));base64,([A-Za-z0-9+/=\r\n]+)$`)

type storedImage struct {
	Mime    string
	Ext     string
	Payload []byte
}

func decodeImageDataURL(dataURL string, allowed map[string]string) (storedImage, error) {
	match := imageDataURLPattern.FindStringSubmatch(strings.TrimSpace(dataURL))
	if match == nil {
		return storedImage{}, fmt.Errorf("invalid image data url")
	}
	mime := normalizeImageMIME(match[1])
	ext, ok := allowed[mime]
	if !ok {
		return storedImage{}, fmt.Errorf("unsupported image mime type")
	}
	payload, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(strings.ReplaceAll(match[3], "\r", ""), "\n", ""))
	if err != nil {
		return storedImage{}, err
	}
	if len(payload) == 0 {
		return storedImage{}, fmt.Errorf("empty image data")
	}
	if !isValidImagePayload(mime, payload) {
		return storedImage{}, fmt.Errorf("image payload does not match mime type")
	}
	return storedImage{Mime: mime, Ext: ext, Payload: payload}, nil
}

func normalizeImageMIME(mime string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	if mime == "image/jpg" {
		return "image/jpeg"
	}
	return mime
}

func isValidImagePayload(mime string, payload []byte) bool {
	switch mime {
	case "image/png":
		return bytes.HasPrefix(payload, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	case "image/jpeg":
		return len(payload) >= 3 && payload[0] == 0xff && payload[1] == 0xd8 && payload[2] == 0xff
	case "image/webp":
		return len(payload) >= 12 && string(payload[0:4]) == "RIFF" && string(payload[8:12]) == "WEBP"
	case "image/gif":
		return bytes.HasPrefix(payload, []byte("GIF87a")) || bytes.HasPrefix(payload, []byte("GIF89a"))
	default:
		return false
	}
}

func imageMIMEFromExt(ext string) (string, bool) {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "png":
		return "image/png", true
	case "jpg", "jpeg":
		return "image/jpeg", true
	case "webp":
		return "image/webp", true
	case "gif":
		return "image/gif", true
	default:
		return "", false
	}
}
