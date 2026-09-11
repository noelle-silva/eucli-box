package filereader

import (
	"fmt"
	"unicode/utf16"
	"unicode/utf8"
)

// decodedText is the text model of one file: the decoded characters plus the
// encoding facts needed to explain non-plain-text files to the caller.
type decodedText struct {
	Text             string
	Encoding         string
	InvalidUTF8      bool
	ReplacementCount int
}

// decodeText turns raw file bytes into text. UTF-8 (with or without BOM) and
// UTF-16 BOM files decode exactly; undecodable bytes are replaced with the
// replacement character and counted so the caller can surface a warning
// instead of silently showing mojibake. Files carrying NUL bytes without a
// UTF-16 marker are rejected as binary content.
func decodeText(data []byte) (decodedText, error) {
	switch {
	case len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF:
		if containsNullByte(data[3:]) {
			return decodedText{}, fmt.Errorf("binary files are not readable as text")
		}
		return decodeUTF8(data[3:], "utf-8-bom"), nil
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE:
		text, err := decodeUTF16(data[2:], true)
		if err != nil {
			return decodedText{}, err
		}
		return decodedText{Text: text, Encoding: "utf-16le"}, nil
	case len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF:
		text, err := decodeUTF16(data[2:], false)
		if err != nil {
			return decodedText{}, err
		}
		return decodedText{Text: text, Encoding: "utf-16be"}, nil
	default:
		if containsNullByte(data) {
			return decodedText{}, fmt.Errorf("binary files are not readable as text")
		}
		return decodeUTF8(data, "utf-8"), nil
	}
}

func decodeUTF8(data []byte, encoding string) decodedText {
	if utf8.Valid(data) {
		return decodedText{Text: string(data), Encoding: encoding}
	}
	runes := make([]rune, 0, len(data))
	count := 0
	for index := 0; index < len(data); {
		decoded, size := utf8.DecodeRune(data[index:])
		if decoded == utf8.RuneError && size == 1 {
			runes = append(runes, utf8.RuneError)
			count++
			index++
			continue
		}
		runes = append(runes, decoded)
		index += size
	}
	return decodedText{Text: string(runes), Encoding: encoding, InvalidUTF8: true, ReplacementCount: count}
}

func decodeUTF16(data []byte, littleEndian bool) (string, error) {
	if len(data)%2 != 0 {
		return "", fmt.Errorf("utf-16 text has an odd byte length")
	}
	units := make([]uint16, 0, len(data)/2)
	for index := 0; index+1 < len(data); index += 2 {
		if littleEndian {
			units = append(units, uint16(data[index])|uint16(data[index+1])<<8)
		} else {
			units = append(units, uint16(data[index])<<8|uint16(data[index+1]))
		}
	}
	return string(utf16.Decode(units)), nil
}
