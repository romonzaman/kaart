package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/romonzaman/kaart/internal/api"
	"github.com/romonzaman/kaart/internal/clock"
	"github.com/romonzaman/kaart/internal/domain"
	"github.com/romonzaman/kaart/internal/scheduler"
	"github.com/romonzaman/kaart/internal/store/sqlite"
)

var testStart = time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC)

type harness struct {
	t      *testing.T
	server *api.Server
	clock  *clock.Fake
}

// newHarness builds a server over a real temp-file SQLite store. Handler tests
// exercise the whole stack — routing, validation, store, scheduler — because
// the seams between them are where the bugs actually live.
func newHarness(t *testing.T) *harness {
	t.Helper()

	path := filepath.Join(t.TempDir(), "api-test.db")
	st, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("closing store: %v", err)
		}
	})

	clk := clock.NewFake(testStart)
	srv := api.New(api.Config{
		Store:       st,
		Clock:       clk,
		Scheduler:   scheduler.NewFactory(),
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		CORSOrigins: []string{"http://localhost:8081"},
		Version:     "test",
	})

	return &harness{t: t, server: srv, clock: clk}
}

func (h *harness) do(method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()

	var reader io.Reader
	if body != nil {
		switch b := body.(type) {
		case string:
			reader = strings.NewReader(b)
		default:
			raw, err := json.Marshal(b)
			if err != nil {
				h.t.Fatalf("marshalling request body: %v", err)
			}
			reader = bytes.NewReader(raw)
		}
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)
	return rec
}

func (h *harness) decode(rec *httptest.ResponseRecorder, dst any) {
	h.t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		h.t.Fatalf("decoding response %q: %v", rec.Body.String(), err)
	}
}

func (h *harness) expectStatus(rec *httptest.ResponseRecorder, want int) {
	h.t.Helper()
	if rec.Code != want {
		h.t.Fatalf("status = %d, want %d (body: %s)", rec.Code, want, rec.Body.String())
	}
}

// expectError asserts the standard error envelope and that no SQL leaked.
func (h *harness) expectError(rec *httptest.ResponseRecorder, status int, code string) {
	h.t.Helper()
	h.expectStatus(rec, status)

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	h.decode(rec, &body)

	if body.Error.Code != code {
		h.t.Fatalf("error code = %q, want %q (body: %s)", body.Error.Code, code, rec.Body.String())
	}
	if body.Error.Message == "" {
		h.t.Fatalf("error message is empty: %s", rec.Body.String())
	}
	for _, leak := range []string{"SELECT", "INSERT", "UPDATE ", "sqlite", "SQL logic error"} {
		if strings.Contains(body.Error.Message, leak) {
			h.t.Fatalf("error message leaks database detail %q: %s", leak, body.Error.Message)
		}
	}
}

func (h *harness) createDeck(name string) map[string]any {
	h.t.Helper()
	rec := h.do(http.MethodPost, "/api/v1/decks", map[string]any{"name": name})
	h.expectStatus(rec, http.StatusCreated)
	var deck map[string]any
	h.decode(rec, &deck)
	return deck
}

func (h *harness) createCard(deckID, front, back string) map[string]any {
	h.t.Helper()
	rec := h.do(http.MethodPost, "/api/v1/decks/"+deckID+"/cards",
		map[string]any{"front": front, "back": back})
	h.expectStatus(rec, http.StatusCreated)
	var card map[string]any
	h.decode(rec, &card)
	return card
}

func TestHealthz(t *testing.T) {
	h := newHarness(t)
	rec := h.do(http.MethodGet, "/healthz", nil)
	h.expectStatus(rec, http.StatusOK)

	var body map[string]any
	h.decode(rec, &body)
	if body["status"] != "ok" {
		t.Fatalf("status = %v, want ok", body["status"])
	}
}

func TestUnknownEndpointReturnsJSONError(t *testing.T) {
	h := newHarness(t)
	h.expectError(h.do(http.MethodGet, "/api/v1/nope", nil), http.StatusNotFound, "not_found")
}

func TestDeckLifecycle(t *testing.T) {
	h := newHarness(t)

	rec := h.do(http.MethodPost, "/api/v1/decks", map[string]any{
		"name":              "  Estonian A1  ",
		"description":       "beginner vocabulary",
		"new_cards_per_day": 10,
		"desired_retention": 0.85,
	})
	h.expectStatus(rec, http.StatusCreated)

	var deck map[string]any
	h.decode(rec, &deck)
	if deck["name"] != "Estonian A1" {
		t.Fatalf("name = %v, want trimmed 'Estonian A1'", deck["name"])
	}
	if deck["new_cards_per_day"].(float64) != 10 {
		t.Fatalf("new_cards_per_day = %v", deck["new_cards_per_day"])
	}
	if deck["max_reviews_per_day"].(float64) != 200 {
		t.Fatalf("max_reviews_per_day = %v, want the default 200", deck["max_reviews_per_day"])
	}
	id := deck["id"].(string)

	rec = h.do(http.MethodGet, "/api/v1/decks/"+id, nil)
	h.expectStatus(rec, http.StatusOK)

	rec = h.do(http.MethodPatch, "/api/v1/decks/"+id, map[string]any{"name": "Estonian A2"})
	h.expectStatus(rec, http.StatusOK)
	h.decode(rec, &deck)
	if deck["name"] != "Estonian A2" {
		t.Fatalf("patched name = %v", deck["name"])
	}
	if deck["new_cards_per_day"].(float64) != 10 {
		t.Fatal("PATCH clobbered a field that was not in the request body")
	}

	rec = h.do(http.MethodGet, "/api/v1/decks", nil)
	h.expectStatus(rec, http.StatusOK)
	var list struct {
		Decks []map[string]any `json:"decks"`
	}
	h.decode(rec, &list)
	if len(list.Decks) != 1 {
		t.Fatalf("deck list length = %d, want 1", len(list.Decks))
	}

	h.expectStatus(h.do(http.MethodDelete, "/api/v1/decks/"+id, nil), http.StatusNoContent)
	h.expectError(h.do(http.MethodGet, "/api/v1/decks/"+id, nil), http.StatusNotFound, "not_found")
}

func TestDeckValidation(t *testing.T) {
	h := newHarness(t)

	tests := []struct {
		name string
		body any
	}{
		{"empty name", map[string]any{"name": "   "}},
		{"missing name", map[string]any{"description": "x"}},
		{"name too long", map[string]any{"name": strings.Repeat("x", 201)}},
		{"unknown field", map[string]any{"name": "ok", "colour": "blue"}},
		{"malformed json", `{"name": `},
		{"empty body", ``},
		{"retention too low", map[string]any{"name": "ok", "desired_retention": 0.1}},
		{"retention too high", map[string]any{"name": "ok", "desired_retention": 1.5}},
		{"negative new cards", map[string]any{"name": "ok", "new_cards_per_day": -1}},
		{"wrong weight count", map[string]any{"name": "ok", "fsrs_weights": []float64{1, 2}}},
		{"wrong type", map[string]any{"name": 42}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h.expectError(h.do(http.MethodPost, "/api/v1/decks", tt.body),
				http.StatusBadRequest, "invalid_request")
		})
	}
}

func TestDeckNotFoundPaths(t *testing.T) {
	h := newHarness(t)

	h.expectError(h.do(http.MethodGet, "/api/v1/decks/missing", nil), http.StatusNotFound, "not_found")
	h.expectError(h.do(http.MethodPatch, "/api/v1/decks/missing", map[string]any{"name": "x"}),
		http.StatusNotFound, "not_found")
	h.expectError(h.do(http.MethodDelete, "/api/v1/decks/missing", nil), http.StatusNotFound, "not_found")
	h.expectError(h.do(http.MethodGet, "/api/v1/decks/missing/cards", nil), http.StatusNotFound, "not_found")
	h.expectError(h.do(http.MethodGet, "/api/v1/decks/missing/queue", nil), http.StatusNotFound, "not_found")
	h.expectError(h.do(http.MethodGet, "/api/v1/decks/missing/stats", nil), http.StatusNotFound, "not_found")
	h.expectError(h.do(http.MethodPost, "/api/v1/decks/missing/cards",
		map[string]any{"front": "a", "back": "b"}), http.StatusNotFound, "not_found")
}

// TestCardRoundTrip is the acceptance criterion from the build plan: create a
// card, fetch it back, and get identical field values including tags.
func TestCardRoundTrip(t *testing.T) {
	h := newHarness(t)
	deck := h.createDeck("Deck")
	deckID := deck["id"].(string)

	sent := map[string]any{
		"front": "  koer  ",
		"back":  "dog",
		"hint":  "KOH-er",
		"tags":  []string{"animals", " a1 ", ""},
	}
	rec := h.do(http.MethodPost, "/api/v1/decks/"+deckID+"/cards", sent)
	h.expectStatus(rec, http.StatusCreated)

	var created map[string]any
	h.decode(rec, &created)
	id := created["id"].(string)

	rec = h.do(http.MethodGet, "/api/v1/cards/"+id, nil)
	h.expectStatus(rec, http.StatusOK)
	var fetched map[string]any
	h.decode(rec, &fetched)

	if fmt.Sprint(created) != fmt.Sprint(fetched) {
		t.Fatalf("created and fetched differ:\n created: %v\n fetched: %v", created, fetched)
	}
	if fetched["front"] != "koer" {
		t.Fatalf("front = %q, want trimmed 'koer'", fetched["front"])
	}
	tags := fetched["tags"].([]any)
	if len(tags) != 2 || tags[0] != "animals" || tags[1] != "a1" {
		t.Fatalf("tags = %v, want [animals a1] with blanks dropped and entries trimmed", tags)
	}
	if fetched["suspended_at"] != nil {
		t.Fatalf("suspended_at = %v, want null", fetched["suspended_at"])
	}
}

func TestCardValidation(t *testing.T) {
	h := newHarness(t)
	deckID := h.createDeck("Deck")["id"].(string)
	path := "/api/v1/decks/" + deckID + "/cards"

	tests := []struct {
		name string
		body any
	}{
		{"empty front", map[string]any{"front": "  ", "back": "b"}},
		{"empty back", map[string]any{"front": "a", "back": ""}},
		{"missing both", map[string]any{}},
		{"tag too long", map[string]any{"front": "a", "back": "b", "tags": []string{strings.Repeat("t", 33)}}},
		{"too many tags", map[string]any{"front": "a", "back": "b", "tags": manyTags(21)}},
		{"unknown field", map[string]any{"front": "a", "back": "b", "colour": "blue"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h.expectError(h.do(http.MethodPost, path, tt.body),
				http.StatusBadRequest, "invalid_request")
		})
	}

	// 20 tags is the limit, not one past it.
	h.expectStatus(h.do(http.MethodPost, path,
		map[string]any{"front": "a", "back": "b", "tags": manyTags(20)}), http.StatusCreated)
}

func TestCardUpdateDeleteAndNotFound(t *testing.T) {
	h := newHarness(t)
	deckID := h.createDeck("Deck")["id"].(string)
	id := h.createCard(deckID, "koer", "dog")["id"].(string)

	rec := h.do(http.MethodPatch, "/api/v1/cards/"+id, map[string]any{"back": "dog (noun)"})
	h.expectStatus(rec, http.StatusOK)
	var patched map[string]any
	h.decode(rec, &patched)
	if patched["back"] != "dog (noun)" {
		t.Fatalf("back = %v", patched["back"])
	}
	if patched["front"] != "koer" {
		t.Fatal("PATCH clobbered front, which was not in the body")
	}

	h.expectError(h.do(http.MethodPatch, "/api/v1/cards/"+id, map[string]any{"front": ""}),
		http.StatusBadRequest, "invalid_request")

	h.expectStatus(h.do(http.MethodDelete, "/api/v1/cards/"+id, nil), http.StatusNoContent)

	h.expectError(h.do(http.MethodGet, "/api/v1/cards/"+id, nil), http.StatusNotFound, "not_found")
	h.expectError(h.do(http.MethodPatch, "/api/v1/cards/missing", map[string]any{"back": "x"}),
		http.StatusNotFound, "not_found")
	h.expectError(h.do(http.MethodDelete, "/api/v1/cards/missing", nil),
		http.StatusNotFound, "not_found")
	h.expectError(h.do(http.MethodPost, "/api/v1/cards/missing/suspend", nil),
		http.StatusNotFound, "not_found")
	h.expectError(h.do(http.MethodPost, "/api/v1/cards/missing/review",
		map[string]any{"rating": 3}), http.StatusNotFound, "not_found")
}

func TestListCardsSearchAndPaging(t *testing.T) {
	h := newHarness(t)
	deckID := h.createDeck("Deck")["id"].(string)

	for i := 0; i < 4; i++ {
		h.clock.Advance(time.Minute)
		h.createCard(deckID, fmt.Sprintf("word%d", i), "meaning")
	}
	h.clock.Advance(time.Minute)
	h.createCard(deckID, "erilinesõna", "special word")

	rec := h.do(http.MethodGet, "/api/v1/decks/"+deckID+"/cards?limit=2&offset=1", nil)
	h.expectStatus(rec, http.StatusOK)
	var page struct {
		Cards []map[string]any `json:"cards"`
		Total int              `json:"total"`
	}
	h.decode(rec, &page)
	if page.Total != 5 || len(page.Cards) != 2 {
		t.Fatalf("total = %d, page = %d, want 5/2", page.Total, len(page.Cards))
	}
	if page.Cards[0]["front"] != "word1" {
		t.Fatalf("offset ignored: first card is %v", page.Cards[0]["front"])
	}

	rec = h.do(http.MethodGet, "/api/v1/decks/"+deckID+"/cards?q=erilines", nil)
	h.expectStatus(rec, http.StatusOK)
	h.decode(rec, &page)
	if page.Total != 1 {
		t.Fatalf("search total = %d, want 1", page.Total)
	}

	h.expectError(h.do(http.MethodGet, "/api/v1/decks/"+deckID+"/cards?limit=abc", nil),
		http.StatusBadRequest, "invalid_request")
	h.expectError(h.do(http.MethodGet, "/api/v1/decks/"+deckID+"/cards?limit=0", nil),
		http.StatusBadRequest, "invalid_request")
}

func TestSuspendBlocksReview(t *testing.T) {
	h := newHarness(t)
	deckID := h.createDeck("Deck")["id"].(string)
	id := h.createCard(deckID, "koer", "dog")["id"].(string)

	rec := h.do(http.MethodPost, "/api/v1/cards/"+id+"/suspend", nil)
	h.expectStatus(rec, http.StatusOK)
	var card map[string]any
	h.decode(rec, &card)
	if card["suspended_at"] == nil {
		t.Fatal("suspended_at should be set after suspend")
	}

	h.expectError(h.do(http.MethodPost, "/api/v1/cards/"+id+"/review",
		map[string]any{"rating": 3}), http.StatusConflict, "conflict")

	// A suspended card is out of the queue.
	rec = h.do(http.MethodGet, "/api/v1/decks/"+deckID+"/queue", nil)
	h.expectStatus(rec, http.StatusOK)
	var queue struct {
		Items []map[string]any `json:"items"`
	}
	h.decode(rec, &queue)
	if len(queue.Items) != 0 {
		t.Fatalf("queue has %d items, want 0 while the only card is suspended", len(queue.Items))
	}

	rec = h.do(http.MethodPost, "/api/v1/cards/"+id+"/unsuspend", nil)
	h.expectStatus(rec, http.StatusOK)
	h.decode(rec, &card)
	if card["suspended_at"] != nil {
		t.Fatal("suspended_at should be null after unsuspend")
	}
	h.expectStatus(h.do(http.MethodPost, "/api/v1/cards/"+id+"/review",
		map[string]any{"rating": 3, "duration_ms": 1500}), http.StatusOK)
}

func TestQueueCarriesRealPreviewIntervals(t *testing.T) {
	h := newHarness(t)
	deckID := h.createDeck("Deck")["id"].(string)
	h.createCard(deckID, "koer", "dog")

	rec := h.do(http.MethodGet, "/api/v1/decks/"+deckID+"/queue?limit=10", nil)
	h.expectStatus(rec, http.StatusOK)

	var queue struct {
		Items []struct {
			Card     map[string]any `json:"card"`
			Previews []struct {
				Rating          int    `json:"rating"`
				RatingName      string `json:"rating_name"`
				Label           string `json:"label"`
				IntervalSeconds int64  `json:"interval_seconds"`
			} `json:"previews"`
		} `json:"items"`
		Counts struct {
			Total int `json:"total"`
			New   int `json:"new"`
		} `json:"counts"`
	}
	h.decode(rec, &queue)

	if len(queue.Items) != 1 {
		t.Fatalf("queue length = %d, want 1", len(queue.Items))
	}
	if queue.Counts.Total != 1 || queue.Counts.New != 1 {
		t.Fatalf("counts = %+v, want one new card", queue.Counts)
	}

	previews := queue.Items[0].Previews
	if len(previews) != 4 {
		t.Fatalf("previews = %d, want 4", len(previews))
	}
	wantOrder := []string{"again", "hard", "good", "easy"}
	for i, p := range previews {
		if p.Rating != i+1 || p.RatingName != wantOrder[i] {
			t.Fatalf("preview %d = %+v, want rating %d (%s)", i, p, i+1, wantOrder[i])
		}
		if p.Label == "" {
			t.Fatalf("preview %s has no label", p.RatingName)
		}
	}
	if previews[3].IntervalSeconds <= previews[0].IntervalSeconds {
		t.Fatalf("easy interval %ds should exceed again interval %ds",
			previews[3].IntervalSeconds, previews[0].IntervalSeconds)
	}
}

// TestQueueEnforcesDailyNewLimit is the acceptance criterion: with
// new_cards_per_day=2, a third new card must not appear.
func TestQueueEnforcesDailyNewLimit(t *testing.T) {
	h := newHarness(t)

	rec := h.do(http.MethodPost, "/api/v1/decks",
		map[string]any{"name": "Deck", "new_cards_per_day": 2})
	h.expectStatus(rec, http.StatusCreated)
	var deck map[string]any
	h.decode(rec, &deck)
	deckID := deck["id"].(string)

	var ids []string
	for i := 0; i < 4; i++ {
		h.clock.Advance(time.Second)
		ids = append(ids, h.createCard(deckID, fmt.Sprintf("w%d", i), "x")["id"].(string))
	}

	if got := h.queueLength(deckID); got != 2 {
		t.Fatalf("queue length = %d, want 2 (the daily new-card allowance)", got)
	}

	// Reviewing the two allowed new cards uses up the allowance for the day.
	for _, id := range ids[:2] {
		h.expectStatus(h.do(http.MethodPost, "/api/v1/cards/"+id+"/review",
			map[string]any{"rating": 3}), http.StatusOK)
	}

	rec = h.do(http.MethodGet, "/api/v1/decks/"+deckID+"/queue", nil)
	h.expectStatus(rec, http.StatusOK)
	var queue struct {
		Counts struct {
			New int `json:"new"`
		} `json:"counts"`
	}
	h.decode(rec, &queue)
	if queue.Counts.New != 0 {
		t.Fatalf("new cards in queue = %d, want 0 once the daily allowance is spent", queue.Counts.New)
	}
}

func (h *harness) queueLength(deckID string) int {
	h.t.Helper()
	rec := h.do(http.MethodGet, "/api/v1/decks/"+deckID+"/queue", nil)
	h.expectStatus(rec, http.StatusOK)
	var queue struct {
		Items []json.RawMessage `json:"items"`
	}
	h.decode(rec, &queue)
	return len(queue.Items)
}

// TestReviewStoresPreReviewState is the acceptance criterion for the log: the
// reviews row must record the state the card was in *before* the rating.
func TestReviewStoresPreReviewState(t *testing.T) {
	h := newHarness(t)
	deckID := h.createDeck("Deck")["id"].(string)
	id := h.createCard(deckID, "koer", "dog")["id"].(string)

	rec := h.do(http.MethodPost, "/api/v1/cards/"+id+"/review",
		map[string]any{"rating": 3, "duration_ms": 2400})
	h.expectStatus(rec, http.StatusOK)

	var resp struct {
		CardID  string `json:"card_id"`
		NextDue string `json:"next_due"`
		State   struct {
			State  string `json:"state"`
			Reps   int    `json:"reps"`
			Lapses int    `json:"lapses"`
		} `json:"state"`
	}
	h.decode(rec, &resp)

	if resp.CardID != id {
		t.Fatalf("card_id = %q", resp.CardID)
	}
	if resp.State.State != "learning" {
		t.Fatalf("state after Good on a new card = %q, want learning", resp.State.State)
	}
	if resp.State.Reps != 1 {
		t.Fatalf("reps = %d, want 1", resp.State.Reps)
	}

	nextDue, err := time.Parse(time.RFC3339Nano, resp.NextDue)
	if err != nil {
		t.Fatalf("parsing next_due %q: %v", resp.NextDue, err)
	}
	if !nextDue.After(h.clock.Now()) {
		t.Fatalf("next_due %v is not in the future", nextDue)
	}

	// The stats endpoint counts the review, and counts it as a new-card intro
	// because the logged pre-review state was New.
	rec = h.do(http.MethodGet, "/api/v1/decks/"+deckID+"/stats", nil)
	h.expectStatus(rec, http.StatusOK)
	var stats struct {
		ReviewsToday      int     `json:"reviews_today"`
		NewCardsToday     int     `json:"new_cards_today"`
		RemainingNewToday int     `json:"remaining_new_today"`
		TotalCards        int     `json:"total_cards"`
		NextDue           *string `json:"next_due"`
		Histogram         []struct {
			Date  string `json:"date"`
			Count int    `json:"count"`
		} `json:"histogram"`
	}
	h.decode(rec, &stats)

	if stats.ReviewsToday != 1 {
		t.Fatalf("reviews_today = %d, want 1", stats.ReviewsToday)
	}
	if stats.NewCardsToday != 1 {
		t.Fatalf("new_cards_today = %d, want 1 — the review log did not store the pre-review state",
			stats.NewCardsToday)
	}
	if stats.RemainingNewToday != 19 {
		t.Fatalf("remaining_new_today = %d, want 19", stats.RemainingNewToday)
	}
	if stats.TotalCards != 1 {
		t.Fatalf("total_cards = %d, want 1", stats.TotalCards)
	}
	if len(stats.Histogram) != 1 || stats.Histogram[0].Count != 1 {
		t.Fatalf("histogram = %v, want one day with one review", stats.Histogram)
	}

	// The card just moved into learning, so it is scheduled ahead of now and
	// the study screen can say when to come back.
	if stats.NextDue == nil {
		t.Fatal("next_due = null, want the learning card's due time")
	}
	statsNextDue, err := time.Parse(time.RFC3339Nano, *stats.NextDue)
	if err != nil {
		t.Fatalf("parsing next_due %q: %v", *stats.NextDue, err)
	}
	if !statsNextDue.After(h.clock.Now()) {
		t.Fatalf("next_due %v is not in the future", statsNextDue)
	}
}

func TestStatsNextDueIsNullWhenEverythingIsDue(t *testing.T) {
	h := newHarness(t)
	deckID := h.createDeck("Deck")["id"].(string)
	h.createCard(deckID, "koer", "dog")

	rec := h.do(http.MethodGet, "/api/v1/decks/"+deckID+"/stats", nil)
	h.expectStatus(rec, http.StatusOK)

	var stats struct {
		DueNow  int     `json:"due_now"`
		NextDue *string `json:"next_due"`
	}
	h.decode(rec, &stats)

	if stats.DueNow != 1 {
		t.Fatalf("due_now = %d, want 1", stats.DueNow)
	}
	if stats.NextDue != nil {
		t.Fatalf("next_due = %v, want null when the only card is already due", *stats.NextDue)
	}
}

func TestReviewValidation(t *testing.T) {
	h := newHarness(t)
	deckID := h.createDeck("Deck")["id"].(string)
	id := h.createCard(deckID, "koer", "dog")["id"].(string)
	path := "/api/v1/cards/" + id + "/review"

	tests := []struct {
		name string
		body any
	}{
		{"rating zero", map[string]any{"rating": 0}},
		{"rating five", map[string]any{"rating": 5}},
		{"missing rating", map[string]any{"duration_ms": 100}},
		{"negative duration", map[string]any{"rating": 3, "duration_ms": -1}},
		{"absurd duration", map[string]any{"rating": 3, "duration_ms": 99999999}},
		{"unknown field", map[string]any{"rating": 3, "confidence": 0.5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h.expectError(h.do(http.MethodPost, path, tt.body),
				http.StatusBadRequest, "invalid_request")
		})
	}
}

// TestRepeatedReviewsAdvanceTheSchedule walks a card from New to Review through
// the HTTP layer, which is the only place the store, scheduler, and clock are
// wired together in production.
func TestRepeatedReviewsAdvanceTheSchedule(t *testing.T) {
	h := newHarness(t)
	deckID := h.createDeck("Deck")["id"].(string)
	id := h.createCard(deckID, "koer", "dog")["id"].(string)

	var lastState string
	for i := 0; i < 8; i++ {
		rec := h.do(http.MethodPost, "/api/v1/cards/"+id+"/review", map[string]any{"rating": 3})
		h.expectStatus(rec, http.StatusOK)

		var resp struct {
			NextDue string `json:"next_due"`
			State   struct {
				State string `json:"state"`
				Reps  int    `json:"reps"`
			} `json:"state"`
		}
		h.decode(rec, &resp)

		if resp.State.Reps != i+1 {
			t.Fatalf("review %d: reps = %d, want %d", i, resp.State.Reps, i+1)
		}
		lastState = resp.State.State

		due, err := time.Parse(time.RFC3339Nano, resp.NextDue)
		if err != nil {
			t.Fatalf("parsing next_due: %v", err)
		}
		h.clock.Set(due) // study the moment it comes due
	}

	if lastState != "review" {
		t.Fatalf("state after eight Good ratings = %q, want review", lastState)
	}
}

func TestCORSAllowsConfiguredOriginOnly(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/decks", nil)
	req.Header.Set("Origin", "http://localhost:8081")
	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8081" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want the configured origin", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/decks", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q for an unlisted origin, want empty", got)
	}

	req = httptest.NewRequest(http.MethodOptions, "/api/v1/decks", nil)
	req.Header.Set("Origin", "http://localhost:8081")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec = httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
}

func TestRequestIDIsEchoed(t *testing.T) {
	h := newHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-Id", "abc-123")
	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-Id"); got != "abc-123" {
		t.Fatalf("X-Request-Id = %q, want the inbound value", got)
	}

	rec = httptest.NewRecorder()
	h.server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatal("a request without X-Request-Id should still get one assigned")
	}
}

func TestWrongMethodIsNotFoundNotPanic(t *testing.T) {
	h := newHarness(t)
	// ServeMux method patterns make this a 405; either way it must be JSON.
	rec := h.do(http.MethodDelete, "/healthz", nil)
	if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 or 405", rec.Code)
	}
}

func manyTags(n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, fmt.Sprintf("tag%d", i))
	}
	return out
}

// The domain enums the API serialises must stay in sync with the strings the
// tests above assert on.
func TestRatingNamesMatchDomain(t *testing.T) {
	want := map[domain.Rating]string{
		domain.RatingAgain: "again",
		domain.RatingHard:  "hard",
		domain.RatingGood:  "good",
		domain.RatingEasy:  "easy",
	}
	for rating, name := range want {
		if rating.String() != name {
			t.Fatalf("Rating(%d).String() = %q, want %q", rating, rating.String(), name)
		}
	}
}
