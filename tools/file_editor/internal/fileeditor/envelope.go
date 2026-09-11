package fileeditor

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

func boolFact(key string, value bool) resultFact {
	if value {
		return resultFact{Key: key, Value: "true"}
	}
	return resultFact{Key: key, Value: "false"}
}

// envelopeLine renders the fixed fact line that ends every result payload.
func envelopeLine(action string, facts []resultFact) string {
	parts := []string{"[file_editor]", "action=" + action}
	for _, fact := range facts {
		if strings.TrimSpace(fact.Value) == "" {
			continue
		}
		parts = append(parts, fact.Key+"="+fact.Value)
	}
	return strings.Join(parts, " ")
}

// appendEnvelope joins a result payload with its fact line.
func appendEnvelope(payload string, action string, facts []resultFact) string {
	envelope := envelopeLine(action, facts)
	body := strings.TrimRight(payload, "\n")
	if body == "" {
		return envelope
	}
	return body + "\n" + envelope
}
