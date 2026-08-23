package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/romonzaman/kaart/internal/domain"
)

// maxBodyBytes caps request bodies. A card is a few hundred bytes; a megabyte
// is generous and stops a malformed client from exhausting memory.
const maxBodyBytes = 1 << 20

// decodeJSON reads and validates a JSON request body. Unknown fields are
// rejected so a typo'd field name fails loudly instead of silently doing
// nothing.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" {
		if mt := strings.TrimSpace(strings.Split(ct, ";")[0]); mt != "application/json" {
			return badRequest("Content-Type must be application/json, got %q", mt)
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return badRequest("request body must contain a single JSON object")
	}
	return nil
}

func decodeError(err error) error {
	var (
		syntaxErr *json.SyntaxError
		typeErr   *json.UnmarshalTypeError
		maxErr    *http.MaxBytesError
	)

	switch {
	case errors.As(err, &syntaxErr):
		return badRequest("request body contains malformed JSON at byte %d", syntaxErr.Offset)
	case errors.As(err, &typeErr):
		if typeErr.Field != "" {
			return badRequest("field %q has the wrong type: expected %s", typeErr.Field, typeErr.Type)
		}
		return badRequest("request body has the wrong type: expected %s", typeErr.Type)
	case errors.Is(err, io.EOF):
		return badRequest("request body is empty")
	case errors.Is(err, io.ErrUnexpectedEOF):
		return badRequest("request body ended unexpectedly")
	case errors.As(err, &maxErr):
		return badRequest("request body is larger than %d bytes", maxBodyBytes)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		field := strings.TrimPrefix(err.Error(), "json: unknown field ")
		return badRequest("unknown field %s", field)
	default:
		return badRequest("request body could not be parsed")
	}
}

// writeJSON renders v with the given status.
func writeJSON(w http.ResponseWriter, r *http.Request, logger *slog.Logger, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logger.ErrorContext(r.Context(), "writing response", slog.String("error", err.Error()))
	}
}

// --- validation ---

// Validation bounds. Kept here so the messages and the checks cannot drift.
const (
	maxDeckNameLen        = 200
	maxDeckDescriptionLen = 2000
	maxCardTextLen        = 10000
	maxTags               = 20
	maxTagLen             = 32
	maxNewCardsPerDay     = 1000
	maxReviewsPerDay      = 10000
	minDesiredRetention   = 0.7
	maxDesiredRetention   = 0.99
)

func validateDeckName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", badRequest("name must not be empty")
	case utf8.RuneCountInString(name) > maxDeckNameLen:
		return "", badRequest("name must be at most %d characters", maxDeckNameLen)
	}
	return name, nil
}

func validateDeckDescription(desc string) (string, error) {
	desc = strings.TrimSpace(desc)
	if utf8.RuneCountInString(desc) > maxDeckDescriptionLen {
		return "", badRequest("description must be at most %d characters", maxDeckDescriptionLen)
	}
	return desc, nil
}

func validateCardText(field, s string) (string, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return "", badRequest("%s must not be empty", field)
	case utf8.RuneCountInString(s) > maxCardTextLen:
		return "", badRequest("%s must be at most %d characters", field, maxCardTextLen)
	}
	return s, nil
}

func validateHint(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxCardTextLen {
		return "", badRequest("hint must be at most %d characters", maxCardTextLen)
	}
	return s, nil
}

// validateTags trims, drops blanks, and rejects oversized sets. It always
// returns a non-nil slice so tags round-trip as [] rather than null.
func validateTags(tags []string) ([]string, error) {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > maxTagLen {
			return nil, badRequest("tag %q must be at most %d characters", t, maxTagLen)
		}
		out = append(out, t)
	}
	if len(out) > maxTags {
		return nil, badRequest("a card may have at most %d tags, got %d", maxTags, len(out))
	}
	return out, nil
}

func validateNewCardsPerDay(n int) error {
	if n < 0 || n > maxNewCardsPerDay {
		return badRequest("new_cards_per_day must be between 0 and %d", maxNewCardsPerDay)
	}
	return nil
}

func validateMaxReviewsPerDay(n int) error {
	if n < 0 || n > maxReviewsPerDay {
		return badRequest("max_reviews_per_day must be between 0 and %d", maxReviewsPerDay)
	}
	return nil
}

func validateDesiredRetention(v float64) error {
	if v < minDesiredRetention || v > maxDesiredRetention {
		return badRequest("desired_retention must be between %.2f and %.2f",
			minDesiredRetention, maxDesiredRetention)
	}
	return nil
}

func validateWeights(w []float64) error {
	if len(w) == 0 {
		return nil
	}
	if len(w) != domain.FSRSWeightCount {
		return badRequest("fsrs_weights must contain exactly %d values, got %d",
			domain.FSRSWeightCount, len(w))
	}
	return nil
}

// --- query parameters ---

// intParam reads a bounded integer query parameter, falling back to def when
// absent or empty.
func intParam(r *http.Request, name string, def, minVal, maxVal int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, badRequest("%s must be an integer", name)
	}
	if n < minVal || n > maxVal {
		return 0, badRequest("%s must be between %d and %d", name, minVal, maxVal)
	}
	return n, nil
}

// pathValue reads a required path segment.
func pathValue(r *http.Request, name string) (string, error) {
	v := strings.TrimSpace(r.PathValue(name))
	if v == "" {
		return "", badRequest("%s is required", name)
	}
	return v, nil
}
