// Package config resolves kaartd's settings from an environment file.
//
// Precedence runs flag > process environment > env file > built-in default.
// The env file is the lowest-priority source on purpose: on a server systemd
// supplies the same keys through EnvironmentFile, and a value already present
// in the process environment must win over a stale file left on disk.
package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// LoadEnvFile reads KEY=VALUE pairs from path into the process environment.
//
// Keys already set in the environment are left alone. A missing file is not an
// error — running from a checkout without a .env is the normal case — but an
// unreadable or malformed one is, because silently serving on the wrong port is
// worse than refusing to start.
func LoadEnvFile(path string) error {
	f, err := os.Open(path) //nolint:gosec // the path is operator-supplied config, not user input
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("opening env file %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only file

	vars, err := parseEnv(f, path)
	if err != nil {
		return err
	}
	for k, v := range vars {
		if _, ok := os.LookupEnv(k); ok {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			return fmt.Errorf("setting %s from %s: %w", k, path, err)
		}
	}
	return nil
}

// parseEnv reads the dotenv subset systemd's EnvironmentFile also accepts, so
// one file can feed both the binary directly and the unit.
func parseEnv(r io.Reader, name string) (map[string]string, error) {
	out := make(map[string]string)
	sc := bufio.NewScanner(r)

	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(strings.TrimSuffix(sc.Text(), "\r"))
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		text = strings.TrimPrefix(text, "export ")

		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected KEY=VALUE, got %q", name, line, text)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("%s:%d: empty key", name, line)
		}

		v, err := unquote(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %s: %w", name, line, key, err)
		}
		out[key] = v
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading env file %s: %w", name, err)
	}
	return out, nil
}

// unquote resolves a value's quoting. Double quotes honour escapes, single
// quotes are literal, and an unquoted value loses any trailing ` # comment`.
//
// A value that opens with a quote it never closes is an error rather than a
// literal: silently keeping the leading quote would put it in the listen
// address or the database path, and the failure would surface far from here.
func unquote(v string) (string, error) {
	for _, q := range []byte{'"', '\''} {
		if len(v) == 0 || v[0] != q {
			continue
		}
		if len(v) < 2 || v[len(v)-1] != q {
			return "", fmt.Errorf("value opens with %c but never closes it", q)
		}
		if q == '\'' {
			return v[1 : len(v)-1], nil
		}
		s, err := strconv.Unquote(v)
		if err != nil {
			return "", fmt.Errorf("malformed quoted value: %w", err)
		}
		return s, nil
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v), nil
}

// String returns the environment value for key, or def when it is unset or empty.
func String(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// List splits a comma-separated environment value, dropping empty entries.
// Returns nil when the key is unset, which callers read as "use the default".
func List(key string) []string {
	raw := String(key, "")
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
