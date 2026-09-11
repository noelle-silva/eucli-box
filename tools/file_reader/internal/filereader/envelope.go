package filereader

import (
	"strconv"
	"strings"
)

// resultFact is one model-visible fact appended to a tool result.
type resultFact struct {
	Key   string
	Value string
}

func textFact(key string, value string) resultFact {
	return resultFact{Key: key, Value: value}
}

func intFact(key string, value int) resultFact {
	return resultFact{Key: key, Value: strconv.Itoa(value)}
}

func int64Fact(key string, value int64) resultFact {
	return resultFact{Key: key, Value: strconv.FormatInt(value, 10)}
}

func boolFact(key string, value bool) resultFact {
	if value {
		return resultFact{Key: key, Value: "true"}
	}
	return resultFact{Key: key, Value: "false"}
}

// envelopeLine renders the fixed fact line that ends every result payload.
func envelopeLine(action string, facts []resultFact) string {
	parts := []string{"[file_reader]", "action=" + action}
	for _, fact := range facts {
		if strings.TrimSpace(fact.Value) == "" {
			continue
		}
		parts = append(parts, fact.Key+"="+fact.Value)
	}
	return strings.Join(parts, " ")
}

// composeContent joins payload and envelope, reserving room for the envelope
// so the model-visible facts survive output truncation. The returned flag
// reports whether the payload itself was cut.
func composeContent(payload string, action string, facts []resultFact, maxOutput int) (string, bool) {
	envelope := envelopeLine(action, facts)
	body, truncated := truncateWithin(payload, maxOutput-len(envelope)-1)
	if truncated {
		envelope = envelopeLine(action, withFact(facts, "truncated", "true"))
	}
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return envelope, truncated
	}
	return body + "\n" + envelope, truncated
}

func withFact(facts []resultFact, key string, value string) []resultFact {
	out := make([]resultFact, len(facts))
	copy(out, facts)
	for index := range out {
		if out[index].Key == key {
			out[index].Value = value
			return out
		}
	}
	return append(out, resultFact{Key: key, Value: value})
}

func truncateWithin(text string, limit int) (string, bool) {
	if limit <= 0 {
		return "", strings.TrimRight(text, "\n") != ""
	}
	return truncateText(text, limit)
}
