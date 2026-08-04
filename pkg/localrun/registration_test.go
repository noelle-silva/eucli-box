package localrun

import (
	"testing"
	"time"
)

func TestMatchRegistrationRequiresAllRuntimeFacts(t *testing.T) {
	installID := mustIdentity(t, IdentityKindInstall)
	dataID := mustIdentity(t, IdentityKindData)
	runID := mustIdentity(t, IdentityKindRun)
	credential := mustIdentity(t, IdentityKindSession)
	startedAt := time.Now().UTC().Truncate(time.Nanosecond)
	registration := Registration{
		SchemaVersion: RegistrationSchemaVersion, InstallIdentity: installID, DataIdentity: dataID,
		RunIdentity: runID, Endpoint: "http://127.0.0.1:40123", SessionCredential: credential,
		ProcessID: 1234, ProcessStartedAt: startedAt, BoxVersion: "0.1.0", Status: RegistrationStatusRunning,
	}
	facts := RegistrationFacts{
		InstallIdentity: installID, DataIdentity: dataID, RunIdentity: runID, Endpoint: registration.Endpoint,
		SessionCredential: credential, ProcessID: 1234, ProcessStartedAt: startedAt, BoxVersion: "0.1.0",
	}
	if err := MatchRegistration(registration, facts); err != nil {
		t.Fatalf("MatchRegistration() error = %v", err)
	}
	facts.SessionCredential = mustIdentity(t, IdentityKindSession)
	if err := MatchRegistration(registration, facts); err == nil {
		t.Fatal("MatchRegistration() accepted a different session credential")
	}
}

func TestValidateRegistrationRejectsNonLoopbackEndpoint(t *testing.T) {
	registration := validRegistrationForTest(t)
	registration.Endpoint = "http://127.0.0.1:0"
	if err := ValidateRegistration(registration); err == nil {
		t.Fatal("ValidateRegistration() accepted port zero")
	}
	registration.Endpoint = "http://localhost:40123"
	if err := ValidateRegistration(registration); err == nil {
		t.Fatal("ValidateRegistration() accepted a non-numeric loopback host")
	}
}

func validRegistrationForTest(t *testing.T) Registration {
	t.Helper()
	return Registration{
		SchemaVersion:     RegistrationSchemaVersion,
		InstallIdentity:   mustIdentity(t, IdentityKindInstall),
		DataIdentity:      mustIdentity(t, IdentityKindData),
		RunIdentity:       mustIdentity(t, IdentityKindRun),
		Endpoint:          "http://127.0.0.1:40123",
		SessionCredential: mustIdentity(t, IdentityKindSession),
		ProcessID:         1234, ProcessStartedAt: time.Now().UTC(), BoxVersion: "0.1.0", Status: RegistrationStatusRunning,
	}
}

func mustIdentity(t *testing.T, kind string) string {
	t.Helper()
	value, err := NewIdentity(kind)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
