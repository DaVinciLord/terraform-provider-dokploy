package client

import "testing"

func TestFormatEnvSortsKeysAlphabetically(t *testing.T) {
	got := formatEnv(map[string]string{
		"ZEBRA": "z",
		"ALPHA": "a",
		"BETA":  "b",
	})
	want := "ALPHA=a\nBETA=b\nZEBRA=z"
	if got != want {
		t.Fatalf("formatEnv() = %q, want %q", got, want)
	}
}
