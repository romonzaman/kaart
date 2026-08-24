package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/romonzaman/kaart/internal/config"
)

// writeEnv puts contents in a temp file and returns its path.
func writeEnv(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kaart.env")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing env file: %v", err)
	}
	return path
}

func TestLoadEnvFile(t *testing.T) {
	path := writeEnv(t, `
# a comment, and a blank line above

KAART_ADDR=127.0.0.1:9000
export KAART_DB=/var/lib/kaart/kaart.db
KAART_LOG_LEVEL = warn
KAART_QUOTED="a value	with a tab"
KAART_SINGLE='literal $NOT_EXPANDED'
KAART_TRAILING=value # not part of the value
KAART_EMPTY=
`)

	for _, k := range []string{
		"KAART_ADDR", "KAART_DB", "KAART_LOG_LEVEL",
		"KAART_QUOTED", "KAART_SINGLE", "KAART_TRAILING", "KAART_EMPTY",
	} {
		t.Setenv(k, "") // registers cleanup; Setenv to "" then unset below
		os.Unsetenv(k)  //nolint:errcheck // best effort, restored by t.Setenv's cleanup
	}

	if err := config.LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}

	want := map[string]string{
		"KAART_ADDR":      "127.0.0.1:9000",
		"KAART_DB":        "/var/lib/kaart/kaart.db",
		"KAART_LOG_LEVEL": "warn",
		"KAART_QUOTED":    "a value\twith a tab",
		"KAART_SINGLE":    "literal $NOT_EXPANDED",
		"KAART_TRAILING":  "value",
		"KAART_EMPTY":     "",
	}
	for k, v := range want {
		if got := os.Getenv(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

// The process environment is authoritative: systemd's EnvironmentFile and a
// stale .env in the working directory must not be able to override it.
func TestLoadEnvFileDoesNotOverrideTheEnvironment(t *testing.T) {
	t.Setenv("KAART_ADDR", "127.0.0.1:1234")
	path := writeEnv(t, "KAART_ADDR=0.0.0.0:8080\n")

	if err := config.LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}
	if got := os.Getenv("KAART_ADDR"); got != "127.0.0.1:1234" {
		t.Fatalf("KAART_ADDR = %q, want the environment's value to survive", got)
	}
}

// A checkout without a .env is the normal development case.
func TestLoadEnvFileMissingIsNotAnError(t *testing.T) {
	if err := config.LoadEnvFile(filepath.Join(t.TempDir(), "absent")); err != nil {
		t.Fatalf("LoadEnvFile on a missing file: %v", err)
	}
}

// Refusing to start beats serving on a port nobody intended.
func TestLoadEnvFileRejectsMalformedLines(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{"no equals sign", "KAART_ADDR 127.0.0.1:8080\n"},
		{"empty key", "=8080\n"},
		{"unterminated quote", "KAART_ADDR=\"127.0.0.1:8080\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := config.LoadEnvFile(writeEnv(t, tc.contents)); err == nil {
				t.Fatal("LoadEnvFile succeeded, want an error")
			}
		})
	}
}

func TestString(t *testing.T) {
	t.Setenv("KAART_ADDR", "  127.0.0.1:9000  ")
	if got := config.String("KAART_ADDR", "default"); got != "127.0.0.1:9000" {
		t.Errorf("String trimmed = %q", got)
	}

	t.Setenv("KAART_BLANK", "   ")
	if got := config.String("KAART_BLANK", "default"); got != "default" {
		t.Errorf("String on a blank value = %q, want the default", got)
	}
	if got := config.String("KAART_UNSET_ENTIRELY", "default"); got != "default" {
		t.Errorf("String on an unset key = %q, want the default", got)
	}
}

func TestList(t *testing.T) {
	t.Setenv("KAART_CORS_ORIGINS", "https://a.example, ,https://b.example,")
	got := config.List("KAART_CORS_ORIGINS")
	want := []string{"https://a.example", "https://b.example"}

	if len(got) != len(want) {
		t.Fatalf("List = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List = %v, want %v", got, want)
		}
	}

	if config.List("KAART_CORS_ORIGINS_UNSET") != nil {
		t.Error("List on an unset key must be nil so the caller can tell it apart from empty")
	}
}
