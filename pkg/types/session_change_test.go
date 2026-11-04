package types

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestSessionChangeEventHasFixedFiveFields(t *testing.T) {
	eventType := reflect.TypeOf(SessionChangeEvent{})
	want := []string{"TargetKind", "Location", "ChangeType", "Version", "OccurredAt"}
	if eventType.NumField() != len(want) {
		t.Fatalf("SessionChangeEvent fields = %d, want %d", eventType.NumField(), len(want))
	}
	for index, name := range want {
		if eventType.Field(index).Name != name {
			t.Fatalf("SessionChangeEvent field %d = %s, want %s", index, eventType.Field(index).Name, name)
		}
	}
}

func TestSessionChangeEventRoundTripsWithoutLoss(t *testing.T) {
	occurredAt := time.Date(2026, 11, 5, 12, 30, 45, 123456789, time.UTC)
	locations := []struct {
		name        string
		location    SessionLocation
		locationKey []string
	}{
		{name: "role", location: RoleSessionLocation("role-1", "session-1"), locationKey: []string{"kind", "roleId", "sessionId"}},
		{name: "group", location: GroupSessionLocation("group-1", "session-1"), locationKey: []string{"kind", "groupId", "sessionId"}},
		{name: "workspace", location: WorkspaceSessionLocation("workspace-1", "role-1", "session-1"), locationKey: []string{"kind", "workspaceId", "roleId", "sessionId"}},
	}
	changeTypes := []SessionChangeType{
		SessionChangeMessageAdded,
		SessionChangeMessageDeleted,
		SessionChangeMessageSubtreeDeleted,
		SessionChangeMessageEdited,
		SessionChangeSessionPersisted,
	}
	for _, target := range locations {
		for _, changeType := range changeTypes {
			event := SessionChangeEvent{
				TargetKind: changeType.TargetKind(),
				Location:   target.location,
				ChangeType: changeType,
				Version:    42,
				OccurredAt: occurredAt,
			}
			payload, err := MarshalSessionChangeEvent(event)
			if err != nil {
				t.Fatalf("MarshalSessionChangeEvent(%s, %s) error = %v", target.name, changeType, err)
			}
			var topLevel map[string]json.RawMessage
			if err := json.Unmarshal(payload, &topLevel); err != nil {
				t.Fatalf("decode top level error = %v", err)
			}
			if len(topLevel) != 5 {
				t.Fatalf("event fields = %#v, want exactly five", topLevel)
			}
			for _, key := range []string{"targetKind", "location", "changeType", "version", "occurredAt"} {
				if _, ok := topLevel[key]; !ok {
					t.Fatalf("event is missing field %s: %s", key, payload)
				}
			}
			var locationFields map[string]json.RawMessage
			if err := json.Unmarshal(topLevel["location"], &locationFields); err != nil {
				t.Fatalf("decode location error = %v", err)
			}
			if len(locationFields) != len(target.locationKey) {
				t.Fatalf("location fields = %#v, want %v", locationFields, target.locationKey)
			}
			for _, key := range target.locationKey {
				if _, ok := locationFields[key]; !ok {
					t.Fatalf("location is missing field %s: %s", key, topLevel["location"])
				}
			}

			decoded, err := UnmarshalSessionChangeEvent(payload)
			if err != nil {
				t.Fatalf("UnmarshalSessionChangeEvent(%s, %s) error = %v", target.name, changeType, err)
			}
			if !reflect.DeepEqual(decoded, event) {
				t.Fatalf("event changed across round trip:\n got %#v\nwant %#v", decoded, event)
			}
		}
	}
}

func TestSessionLocationRequiresCompleteMatchingFields(t *testing.T) {
	cases := []struct {
		name     string
		location SessionLocation
	}{
		{name: "role missing role", location: SessionLocation{Kind: SessionLocationRole, SessionID: "session-1"}},
		{name: "role missing session", location: SessionLocation{Kind: SessionLocationRole, RoleID: "role-1"}},
		{name: "role carries group", location: SessionLocation{Kind: SessionLocationRole, RoleID: "role-1", GroupID: "group-1", SessionID: "session-1"}},
		{name: "role carries workspace", location: SessionLocation{Kind: SessionLocationRole, RoleID: "role-1", WorkspaceID: "workspace-1", SessionID: "session-1"}},
		{name: "group missing group", location: SessionLocation{Kind: SessionLocationGroup, SessionID: "session-1"}},
		{name: "group carries role", location: SessionLocation{Kind: SessionLocationGroup, GroupID: "group-1", RoleID: "role-1", SessionID: "session-1"}},
		{name: "workspace missing workspace", location: SessionLocation{Kind: SessionLocationWorkspace, RoleID: "role-1", SessionID: "session-1"}},
		{name: "workspace missing role", location: SessionLocation{Kind: SessionLocationWorkspace, WorkspaceID: "workspace-1", SessionID: "session-1"}},
		{name: "workspace carries group", location: SessionLocation{Kind: SessionLocationWorkspace, WorkspaceID: "workspace-1", RoleID: "role-1", GroupID: "group-1", SessionID: "session-1"}},
		{name: "unknown kind", location: SessionLocation{Kind: "unknown", RoleID: "role-1", SessionID: "session-1"}},
	}
	for _, testCase := range cases {
		if err := testCase.location.Validate(); err == nil {
			t.Fatalf("%s must be rejected: %#v", testCase.name, testCase.location)
		}
	}
}

func TestSessionChangeEventRejectsIncompleteOrMismatchedFacts(t *testing.T) {
	occurredAt := time.Date(2026, 11, 5, 12, 30, 45, 0, time.UTC)
	validLocation := RoleSessionLocation("role-1", "session-1")
	cases := []struct {
		name  string
		event SessionChangeEvent
	}{
		{name: "invalid target", event: SessionChangeEvent{TargetKind: "unknown", Location: validLocation, ChangeType: SessionChangeMessageAdded, Version: 1, OccurredAt: occurredAt}},
		{name: "invalid change type", event: SessionChangeEvent{TargetKind: SessionChangeTargetMessage, Location: validLocation, ChangeType: "unknown", Version: 1, OccurredAt: occurredAt}},
		{name: "mismatched target", event: SessionChangeEvent{TargetKind: SessionChangeTargetSession, Location: validLocation, ChangeType: SessionChangeMessageAdded, Version: 1, OccurredAt: occurredAt}},
		{name: "incomplete location", event: SessionChangeEvent{TargetKind: SessionChangeTargetMessage, Location: SessionLocation{Kind: SessionLocationRole, SessionID: "session-1"}, ChangeType: SessionChangeMessageAdded, Version: 1, OccurredAt: occurredAt}},
		{name: "missing version", event: SessionChangeEvent{TargetKind: SessionChangeTargetMessage, Location: validLocation, ChangeType: SessionChangeMessageAdded, OccurredAt: occurredAt}},
		{name: "missing occurrence time", event: SessionChangeEvent{TargetKind: SessionChangeTargetMessage, Location: validLocation, ChangeType: SessionChangeMessageAdded, Version: 1}},
	}
	for _, testCase := range cases {
		if err := testCase.event.Validate(); err == nil {
			t.Fatalf("%s must be rejected: %#v", testCase.name, testCase.event)
		}
	}
}

func TestSessionLocationFromSessionCoversEveryForm(t *testing.T) {
	cases := []struct {
		name     string
		session  Session
		expected SessionLocation
	}{
		{name: "role", session: Session{ID: "session-1", RoleID: "role-1"}, expected: RoleSessionLocation("role-1", "session-1")},
		{name: "group", session: Session{ID: "session-1", GroupID: "group-1"}, expected: GroupSessionLocation("group-1", "session-1")},
		{name: "workspace", session: Session{ID: "session-1", WorkspaceID: "workspace-1", RoleID: "role-1"}, expected: WorkspaceSessionLocation("workspace-1", "role-1", "session-1")},
	}
	for _, testCase := range cases {
		location, err := SessionLocationFromSession(testCase.session)
		if err != nil {
			t.Fatalf("%s: SessionLocationFromSession() error = %v", testCase.name, err)
		}
		if location != testCase.expected {
			t.Fatalf("%s: location = %#v, want %#v", testCase.name, location, testCase.expected)
		}
	}

	invalid := []Session{
		{ID: "session-1"},
		{ID: "session-1", GroupID: "group-1", RoleID: "role-1"},
		{ID: "session-1", WorkspaceID: "workspace-1"},
	}
	for _, session := range invalid {
		if _, err := SessionLocationFromSession(session); err == nil {
			t.Fatalf("incomplete session location must be rejected: %#v", session)
		}
	}
}
