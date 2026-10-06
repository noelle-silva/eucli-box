package types

import "testing"

func TestSessionToolAuthorizationRoundTrip(t *testing.T) {
	metadata := PutSessionToolAuthorization(nil, "file-reader", true)
	if !SessionToolAuthorized(metadata, "file-reader") {
		t.Fatalf("tool should be authorized: %#v", metadata)
	}
	if SessionToolAuthorized(metadata, "shell-command") {
		t.Fatalf("other tool must stay unauthorized: %#v", metadata)
	}
}

func TestPutSessionToolAuthorizationRemovesOnFalse(t *testing.T) {
	metadata := PutSessionToolAuthorization(map[string]string{"keep": "1"}, "file-reader", true)
	metadata = PutSessionToolAuthorization(metadata, "file-reader", false)
	if SessionToolAuthorized(metadata, "file-reader") {
		t.Fatalf("tool authorization should be removed: %#v", metadata)
	}
	if metadata["keep"] != "1" {
		t.Fatalf("unrelated metadata must be preserved: %#v", metadata)
	}
}

func TestPutSessionToolAuthorizationIgnoresBlankTool(t *testing.T) {
	metadata := PutSessionToolAuthorization(map[string]string{"keep": "1"}, "  ", true)
	if metadata["keep"] != "1" || len(metadata) != 1 {
		t.Fatalf("blank tool id must not create entries: %#v", metadata)
	}
}

func TestSessionToolAuthorizedIgnoresNonTrueValue(t *testing.T) {
	if SessionToolAuthorized(map[string]string{SessionMetadataToolAuthorizationPrefix + "file-reader": "yes"}, "file-reader") {
		t.Fatalf("only explicit true counts as authorized")
	}
}
