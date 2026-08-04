package localrun

import (
	"strings"
	"testing"
)

func TestNewIdentityUsesFixedOpaqueFormat(t *testing.T) {
	for _, kind := range []string{IdentityKindInstall, IdentityKindData, IdentityKindRun, IdentityKindSession} {
		value, err := NewIdentity(kind)
		if err != nil {
			t.Fatalf("NewIdentity(%q) error = %v", kind, err)
		}
		if !strings.HasPrefix(value, kind+"-") || len(value) != len(kind)+1+64 {
			t.Fatalf("identity %q has unexpected format", value)
		}
		if err := ValidateIdentity(value, kind); err != nil {
			t.Fatalf("ValidateIdentity(%q) error = %v", value, err)
		}
	}
}

func TestValidateIdentityRejectsWrongKindAndMixedCase(t *testing.T) {
	value, err := NewIdentity(IdentityKindRun)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []struct {
		value string
		kind  string
	}{
		{value: value, kind: IdentityKindData},
		{value: strings.ToUpper(value), kind: IdentityKindRun},
		{value: value[:len(value)-1], kind: IdentityKindRun},
	} {
		if err := ValidateIdentity(candidate.value, candidate.kind); err == nil {
			t.Fatalf("ValidateIdentity(%q, %q) unexpectedly succeeded", candidate.value, candidate.kind)
		}
	}
}
