package browser

import "testing"

func TestParseBackendModeAliases(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]BackendMode{
		"":         BackendModeCurrent,
		"auto":     BackendModeAuto,
		"ob":       BackendModeObscura,
		"OBSCURA":  BackendModeObscura,
		"ch":       BackendModeChromium,
		"Chromium": BackendModeChromium,
	} {
		got, err := ParseBackendMode(input)
		if err != nil || got != want {
			t.Fatalf("ParseBackendMode(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := ParseBackendMode("other"); err == nil {
		t.Fatal("invalid backend was accepted")
	}
}
