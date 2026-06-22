package cmd

import "testing"

func TestNormalizeHost(t *testing.T) {
	if got := normalizeHost(""); got != "127.0.0.1" {
		t.Fatalf("expected default host, got %q", got)
	}
	if got := normalizeHost(" 0.0.0.0 "); got != "0.0.0.0" {
		t.Fatalf("expected trimmed host, got %q", got)
	}
}

func TestNormalizePort(t *testing.T) {
	if got := normalizePort(0); got != 9876 {
		t.Fatalf("expected default port, got %d", got)
	}
	if got := normalizePort(4000); got != 4000 {
		t.Fatalf("expected custom port, got %d", got)
	}
}

func TestNormalizeDetector(t *testing.T) {
	cases := map[string]string{
		"":         "auto",
		"auto":     "auto",
		"pm2":      "pm2",
		"systemd":  "systemd",
		"unknown":  "auto",
		" SYSTEMD": "systemd",
	}

	for input, expected := range cases {
		if got := normalizeDetector(input); got != expected {
			t.Fatalf("normalizeDetector(%q) = %q, want %q", input, got, expected)
		}
	}
}
