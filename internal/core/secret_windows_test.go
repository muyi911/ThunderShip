package core

import "testing"

func TestWindowsCredentialRoundTrip(t *testing.T) {
	id := "test/" + ID()
	defer DeleteSecret(id)
	if err := SetSecret(id, "disposable-test-value"); err != nil {
		t.Fatal(err)
	}
	got, err := GetSecret(id)
	if err != nil || got != "disposable-test-value" {
		t.Fatal("credential round trip failed", err)
	}
	if err = DeleteSecret(id); err != nil {
		t.Fatal(err)
	}
	if _, err = GetSecret(id); err == nil {
		t.Fatal("credential was not removed")
	}
}
