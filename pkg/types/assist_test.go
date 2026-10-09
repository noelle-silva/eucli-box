package types

import "testing"

func TestAssistTemperatureResolvesLowTemperatureSwitch(t *testing.T) {
	if got := AssistTemperature(nil); got != nil {
		t.Fatalf("unset switch must not send temperature: %#v", got)
	}

	off := false
	if got := AssistTemperature(&off); got != nil {
		t.Fatalf("disabled switch must not send temperature: %#v", got)
	}

	on := true
	got := AssistTemperature(&on)
	if got == nil || *got != DefaultAssistLowTemperature {
		t.Fatalf("enabled switch must send the low temperature: %#v", got)
	}
}
