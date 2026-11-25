package adapters

import "testing"

func TestIssuerBaseURL(t *testing.T) {
	got, err := issuerBaseURL("https://login.microsoftonline.com/ea649e50-27d2-4318-9cb9-2fcffe16fd41/v2.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "https://login.microsoftonline.com"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := issuerBaseURL("not-a-url"); err == nil {
		t.Fatal("expected error for invalid issuer URL")
	}
}
