package systemplugin

import (
	"testing"
)

func TestValidateReadyHandshake(t *testing.T) {
	expected := []Capability{{Type: CapabilityPlaceholderValues, Interfaces: []string{"a"}}}
	message := NewMessage(MessageReady)
	message.PluginID = "demo"
	message.Capabilities = expected
	if err := ValidateReady(message, "demo", expected); err != nil {
		t.Fatalf("ValidateReady() error = %v", err)
	}
	if err := ValidateReady(message, "other", expected); err == nil {
		t.Fatalf("expected identity mismatch error")
	}
	message.ProtocolVersion = ProtocolVersion + 1
	if err := ValidateReady(message, "demo", expected); err == nil {
		t.Fatalf("expected protocol mismatch error")
	}
}

func TestValidateReadyCapabilities(t *testing.T) {
	expected := []Capability{{Type: CapabilityPlaceholderValues, Interfaces: []string{"a", "b"}}}
	message := NewMessage(MessageReady)
	message.PluginID = "demo"
	message.Capabilities = []Capability{{Type: CapabilityPlaceholderValues, Interfaces: []string{"b", "a"}}}
	if err := ValidateReady(message, "demo", expected); err != nil {
		t.Fatalf("ValidateReady() error = %v", err)
	}
	message.Capabilities = []Capability{{Type: CapabilityPlaceholderValues, Interfaces: []string{"a"}}}
	if err := ValidateReady(message, "demo", expected); err == nil {
		t.Fatalf("expected capability mismatch error")
	}
}

func TestEqualCapabilities(t *testing.T) {
	left := []Capability{{Type: "placeholder-values", Interfaces: []string{"b", "a"}}}
	right := []Capability{{Type: "placeholder-values", Interfaces: []string{"a", "b"}}}
	if !EqualCapabilities(left, right) {
		t.Fatalf("capabilities should be equal regardless of order")
	}
	if EqualCapabilities(left, nil) {
		t.Fatalf("non-empty and empty capabilities must differ")
	}
}
