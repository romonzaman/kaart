package sqlite_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/romonzaman/kaart/internal/domain"
	"github.com/romonzaman/kaart/internal/store"
	"github.com/romonzaman/kaart/internal/store/sqlite"
)

var baseTime = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

// newStore opens a store backed by a real temp-file database. Tests run against
// SQLite itself rather than a mock: the ordering, cascade, and text-comparison
// behaviour being asserted here is SQLite's, not ours.
func newStore(t *testing.T) *sqlite.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kaart-test.db")

	s, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("closing store: %v", err)
		}
	})
	return s
}

func newDeck(id, name string) *domain.Deck {
	return &domain.Deck{
		ID:               id,
		Name:             name,
		Description:      "",
		NewCardsPerDay:   domain.DefaultNewCardsPerDay,
		MaxReviewsPerDay: domain.DefaultMaxReviewsPerDay,
		DesiredRetention: domain.DefaultDesiredRetention,
		CreatedAt:        baseTime,
		UpdatedAt:        baseTime,
	}
}

func mustDeck(t *testing.T, s *sqlite.Store, id, name string) *domain.Deck {
	t.Helper()
	d := newDeck(id, name)
	if err := s.CreateDeck(context.Background(), d); err != nil {
		t.Fatalf("creating deck %s: %v", id, err)
	}
	return d
}

func mustCard(t *testing.T, s *sqlite.Store, deckID, id, front string, created time.Time) *domain.Card {
	t.Helper()
	c := &domain.Card{
		ID:        id,
		DeckID:    deckID,
		Front:     front,
		Back:      front + "-back",
		Tags:      []string{},
		CreatedAt: created,
		UpdatedAt: created,
	}
	st := domain.NewCardState(id, created)
	if err := s.CreateCard(context.Background(), c, &st); err != nil {
		t.Fatalf("creating card %s: %v", id, err)
	}
	return c
}

func TestDeckCRUD(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	d := newDeck("deck-1", "Estonian A1")
	d.Description = "beginner vocabulary"
	d.FSRSWeights = nil
	if err := s.CreateDeck(ctx, d); err != nil {
		t.Fatalf("CreateDeck: %v", err)
	}

	got, err := s.GetDeck(ctx, "deck-1")
	if err != nil {
		t.Fatalf("GetDeck: %v", err)
	}
	if got.Name != "Estonian A1" || got.Description != "beginner vocabulary" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.NewCardsPerDay != 20 || got.MaxReviewsPerDay != 200 || got.DesiredRetention != 0.9 {
		t.Fatalf("settings not persisted: %+v", got)
	}
	if got.FSRSWeights != nil {
		t.Fatalf("FSRSWeights = %v, want nil", got.FSRSWeights)
	}
	if !got.CreatedAt.Equal(baseTime) {
		t.Fatalf("CreatedAt = %v, want %v", got.CreatedAt, baseTime)
	}

	got.Name = "Estonian A1 (revised)"
	got.NewCardsPerDay = 5
	got.FSRSWeights = []float64{0.5, 1.5, 2.5}
	got.UpdatedAt = baseTime.Add(time.Hour)
	if err := s.UpdateDeck(ctx, got); err != nil {
		t.Fatalf("UpdateDeck: %v", err)
	}

	got2, err := s.GetDeck(ctx, "deck-1")
	if err != nil {
		t.Fatalf("GetDeck after update: %v", err)
	}
	if got2.Name != "Estonian A1 (revised)" || got2.NewCardsPerDay != 5 {
		t.Fatalf("update not persisted: %+v", got2)
	}
	if len(got2.FSRSWeights) != 3 || got2.FSRSWeights[2] != 2.5 {
		t.Fatalf("weights not persisted: %v", got2.FSRSWeights)
	}

	if err := s.DeleteDeck(ctx, "deck-1"); err != nil {
		t.Fatalf("DeleteDeck: %v", err)
	}
	if _, err := s.GetDeck(ctx, "deck-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetDeck after delete: err = %v, want ErrNotFound", err)
	}
}

func TestDeckNotFoundPaths(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	if _, err := s.GetDeck(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetDeck: err = %v, want ErrNotFound", err)
	}
	if err := s.UpdateDeck(ctx, newDeck("missing", "x")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("UpdateDeck: err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteDeck(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteDeck: err = %v, want ErrNotFound", err)
	}
}

func TestListDecksExcludesArchivedByDefault(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	mustDeck(t, s, "d-b", "Beta")
	mustDeck(t, s, "d-a", "Alpha")
	archived := mustDeck(t, s, "d-c", "Gamma")
	at := baseTime
	archived.ArchivedAt = &at
	if err := s.UpdateDeck(ctx, archived); err != nil {
		t.Fatalf("archiving deck: %v", err)
	}

	active, err := s.ListDecks(ctx, store.DeckFilter{})
	if err != nil {
		t.Fatalf("ListDecks: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("active decks = %d, want 2", len(active))
	}
	if active[0].Name != "Alpha" || active[1].Name != "Beta" {
		t.Fatalf("decks not ordered by name: %v, %v", active[0].Name, active[1].Name)
	}

	all, err := s.ListDecks(ctx, store.DeckFilter{IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListDecks(all): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("all decks = %d, want 3", len(all))
	}
}

func TestCardCRUDAndTagRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	mustDeck(t, s, "deck-1", "Deck")

	c := &domain.Card{
		ID:        "card-1",
		DeckID:    "deck-1",
		Front:     "koer",
		Back:      "dog",
		Hint:      "KOH-er",
		Tags:      []string{"animals", "a1"},
		CreatedAt: baseTime,
		UpdatedAt: baseTime,
	}
	st := domain.NewCardState(c.ID, baseTime)
	if err := s.CreateCard(ctx, c, &st); err != nil {
		t.Fatalf("CreateCard: %v", err)
	}

	got, err := s.GetCard(ctx, "card-1")
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if got.Front != "koer" || got.Back != "dog" || got.Hint != "KOH-er" {
		t.Fatalf("content mismatch: %+v", got)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "animals" || got.Tags[1] != "a1" {
		t.Fatalf("tags = %v, want [animals a1]", got.Tags)
	}

	// The state row must have been created in the same transaction.
	gotState, err := s.GetCardState(ctx, "card-1")
	if err != nil {
		t.Fatalf("GetCardState: %v", err)
	}
	if gotState.State != domain.StateNew || !gotState.Due.Equal(baseTime) {
		t.Fatalf("initial state = %+v", gotState)
	}

	got.Back = "dog (noun)"
	got.Tags = nil
	got.UpdatedAt = baseTime.Add(time.Minute)
	if err := s.UpdateCard(ctx, got); err != nil {
		t.Fatalf("UpdateCard: %v", err)
	}
	got2, err := s.GetCard(ctx, "card-1")
	if err != nil {
		t.Fatalf("GetCard after update: %v", err)
	}
	if got2.Back != "dog (noun)" {
		t.Fatalf("Back = %q", got2.Back)
	}
	if got2.Tags == nil || len(got2.Tags) != 0 {
		t.Fatalf("Tags = %#v, want empty non-nil slice", got2.Tags)
	}

	if err := s.DeleteCard(ctx, "card-1"); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	if _, err := s.GetCard(ctx, "card-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetCard after delete: %v", err)
	}
}

func TestCardNotFoundPaths(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	if _, err := s.GetCard(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetCard: %v", err)
	}
	if err := s.DeleteCard(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteCard: %v", err)
	}
	if err := s.SetCardSuspended(ctx, "missing", nil); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("SetCardSuspended: %v", err)
	}
	if _, err := s.GetCardState(ctx, "missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("GetCardState: %v", err)
	}

	c := &domain.Card{ID: "orphan", DeckID: "no-such-deck", Front: "a", Back: "b",
		CreatedAt: baseTime, UpdatedAt: baseTime}
	st := domain.NewCardState(c.ID, baseTime)
	if err := s.CreateCard(ctx, c, &st); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("CreateCard into missing deck: err = %v, want ErrNotFound", err)
	}
}

func TestListCardsSearchAndPaging(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	mustDeck(t, s, "deck-1", "Deck")

	for i := 0; i < 5; i++ {
		mustCard(t, s, "deck-1", fmt.Sprintf("card-%d", i),
			fmt.Sprintf("word%d", i), baseTime.Add(time.Duration(i)*time.Minute))
	}
	mustCard(t, s, "deck-1", "card-x", "erilinesõna", baseTime.Add(time.Hour))

	all, total, err := s.ListCards(ctx, "deck-1", store.CardFilter{})
	if err != nil {
		t.Fatalf("ListCards: %v", err)
	}
	if total != 6 || len(all) != 6 {
		t.Fatalf("total = %d, len = %d, want 6/6", total, len(all))
	}
	if all[0].ID != "card-0" || all[5].ID != "card-x" {
		t.Fatalf("cards not ordered by created_at: %s .. %s", all[0].ID, all[5].ID)
	}

	page, total, err := s.ListCards(ctx, "deck-1", store.CardFilter{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("ListCards(page): %v", err)
	}
	if total != 6 {
		t.Fatalf("total with paging = %d, want 6", total)
	}
	if len(page) != 2 || page[0].ID != "card-2" {
		t.Fatalf("page = %v", page)
	}

	found, total, err := s.ListCards(ctx, "deck-1", store.CardFilter{Query: "erilines"})
	if err != nil {
		t.Fatalf("ListCards(query): %v", err)
	}
	if total != 1 || len(found) != 1 || found[0].ID != "card-x" {
		t.Fatalf("search returned %d rows: %v", total, found)
	}

	// A '%' in the query must be matched literally, not treated as a wildcard.
	none, total, err := s.ListCards(ctx, "deck-1", store.CardFilter{Query: "%"})
	if err != nil {
		t.Fatalf("ListCards(wildcard): %v", err)
	}
	if total != 0 || len(none) != 0 {
		t.Fatalf("literal %% search matched %d rows", total)
	}
}

func TestSuspendUnsuspend(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	mustDeck(t, s, "deck-1", "Deck")
	mustCard(t, s, "deck-1", "card-1", "koer", baseTime)

	at := baseTime.Add(time.Hour)
	if err := s.SetCardSuspended(ctx, "card-1", &at); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	got, err := s.GetCard(ctx, "card-1")
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if !got.Suspended() || !got.SuspendedAt.Equal(at) {
		t.Fatalf("SuspendedAt = %v, want %v", got.SuspendedAt, at)
	}

	if err := s.SetCardSuspended(ctx, "card-1", nil); err != nil {
		t.Fatalf("unsuspend: %v", err)
	}
	got, err = s.GetCard(ctx, "card-1")
	if err != nil {
		t.Fatalf("GetCard: %v", err)
	}
	if got.Suspended() {
		t.Fatal("card should not be suspended after unsuspend")
	}
}

func TestDeletingDeckCascades(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	mustDeck(t, s, "deck-1", "Deck")
	mustCard(t, s, "deck-1", "card-1", "koer", baseTime)

	rv := &domain.Review{
		CardID: "card-1", Rating: domain.RatingGood, State: domain.StateNew,
		Due: baseTime, ReviewedAt: baseTime, DurationMS: 1200,
	}
	if err := s.AppendReview(ctx, rv); err != nil {
		t.Fatalf("AppendReview: %v", err)
	}
	if rv.ID == 0 {
		t.Fatal("AppendReview did not set the review ID")
	}

	if err := s.DeleteDeck(ctx, "deck-1"); err != nil {
		t.Fatalf("DeleteDeck: %v", err)
	}

	if _, err := s.GetCard(ctx, "card-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("card survived deck delete: %v", err)
	}
	if _, err := s.GetCardState(ctx, "card-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("card state survived deck delete: %v", err)
	}

	var n int
	if err := s.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reviews WHERE card_id = ?`, "card-1").Scan(&n); err != nil {
		t.Fatalf("counting reviews: %v", err)
	}
	if n != 0 {
		t.Fatalf("reviews survived deck delete: %d rows", n)
	}
}

func TestDeletingCardCascades(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	mustDeck(t, s, "deck-1", "Deck")
	mustCard(t, s, "deck-1", "card-1", "koer", baseTime)

	if err := s.AppendReview(ctx, &domain.Review{
		CardID: "card-1", Rating: domain.RatingGood, State: domain.StateNew,
		Due: baseTime, ReviewedAt: baseTime,
	}); err != nil {
		t.Fatalf("AppendReview: %v", err)
	}

	if err := s.DeleteCard(ctx, "card-1"); err != nil {
		t.Fatalf("DeleteCard: %v", err)
	}
	if _, err := s.GetCardState(ctx, "card-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("card state survived card delete: %v", err)
	}

	var n int
	if err := s.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reviews WHERE card_id = ?`, "card-1").Scan(&n); err != nil {
		t.Fatalf("counting reviews: %v", err)
	}
	if n != 0 {
		t.Fatalf("reviews survived card delete: %d rows", n)
	}

	// The deck itself is untouched.
	if _, err := s.GetDeck(ctx, "deck-1"); err != nil {
		t.Fatalf("deck should survive card delete: %v", err)
	}
}

func TestUpsertCardState(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	mustDeck(t, s, "deck-1", "Deck")
	mustCard(t, s, "deck-1", "card-1", "koer", baseTime)

	last := baseTime.Add(2 * time.Hour)
	st := &domain.CardState{
		CardID: "card-1", Due: baseTime.Add(48 * time.Hour),
		Stability: 3.25, Difficulty: 5.5,
		ElapsedDays: 1, ScheduledDays: 2, Reps: 3, Lapses: 1,
		State: domain.StateReview, RemainingSteps: 0, LastReview: &last,
	}
	if err := s.UpsertCardState(ctx, st); err != nil {
		t.Fatalf("UpsertCardState: %v", err)
	}

	got, err := s.GetCardState(ctx, "card-1")
	if err != nil {
		t.Fatalf("GetCardState: %v", err)
	}
	if got.Stability != 3.25 || got.Difficulty != 5.5 || got.Reps != 3 || got.Lapses != 1 {
		t.Fatalf("state not persisted: %+v", got)
	}
	if got.State != domain.StateReview {
		t.Fatalf("State = %v", got.State)
	}
	if got.LastReview == nil || !got.LastReview.Equal(last) {
		t.Fatalf("LastReview = %v, want %v", got.LastReview, last)
	}
	if !got.Due.Equal(baseTime.Add(48 * time.Hour)) {
		t.Fatalf("Due = %v", got.Due)
	}
}

func TestApplyReviewIsAtomicAndRejectsSuspended(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	mustDeck(t, s, "deck-1", "Deck")
	mustCard(t, s, "deck-1", "card-1", "koer", baseTime)

	newState := &domain.CardState{
		CardID: "card-1", Due: baseTime.Add(10 * time.Minute),
		Stability: 1.5, Difficulty: 5, ScheduledDays: 0, Reps: 1,
		State: domain.StateLearning, LastReview: &baseTime,
	}
	pre := &domain.Review{
		CardID: "card-1", Rating: domain.RatingGood, State: domain.StateNew,
		Due: baseTime, Stability: 0, Difficulty: 0,
		ElapsedDays: 0, LastElapsedDays: 0, ScheduledDays: 0,
		ReviewedAt: baseTime, DurationMS: 2400,
	}
	if err := s.ApplyReview(ctx, newState, pre); err != nil {
		t.Fatalf("ApplyReview: %v", err)
	}

	got, err := s.GetCardState(ctx, "card-1")
	if err != nil {
		t.Fatalf("GetCardState: %v", err)
	}
	if got.State != domain.StateLearning || got.Reps != 1 {
		t.Fatalf("state after review: %+v", got)
	}

	var (
		state, rating int
		stability     float64
	)
	if err := s.DB().QueryRowContext(ctx,
		`SELECT state, rating, stability FROM reviews WHERE card_id = ?`, "card-1").
		Scan(&state, &rating, &stability); err != nil {
		t.Fatalf("reading review row: %v", err)
	}
	if domain.State(state) != domain.StateNew || stability != 0 {
		t.Fatalf("review row stored post-review state (state=%d stability=%v), want pre-review", state, stability)
	}
	if domain.Rating(rating) != domain.RatingGood {
		t.Fatalf("rating = %d", rating)
	}

	at := baseTime
	if err := s.SetCardSuspended(ctx, "card-1", &at); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := s.ApplyReview(ctx, newState, pre); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("ApplyReview on suspended card: err = %v, want ErrConflict", err)
	}
	if err := s.ApplyReview(ctx, newState, &domain.Review{
		CardID: "missing", Rating: domain.RatingGood, Due: baseTime, ReviewedAt: baseTime,
	}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("ApplyReview on missing card: err = %v, want ErrNotFound", err)
	}
}

func TestDueQueueOrdering(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	mustDeck(t, s, "deck-1", "Deck")

	// Created newest-first so creation order can't accidentally produce the
	// expected queue order.
	mustCard(t, s, "deck-1", "new-1", "new one", baseTime.Add(3*time.Minute))
	mustCard(t, s, "deck-1", "new-2", "new two", baseTime.Add(4*time.Minute))
	mustCard(t, s, "deck-1", "review-1", "review one", baseTime.Add(2*time.Minute))
	mustCard(t, s, "deck-1", "learn-1", "learning one", baseTime.Add(time.Minute))
	mustCard(t, s, "deck-1", "future-1", "not due", baseTime)
	mustCard(t, s, "deck-1", "susp-1", "suspended", baseTime)

	now := baseTime.Add(24 * time.Hour)

	setState(t, s, "learn-1", domain.StateLearning, now.Add(-time.Minute))
	setState(t, s, "review-1", domain.StateReview, now.Add(-2*time.Hour))
	setState(t, s, "future-1", domain.StateReview, now.Add(2*time.Hour))
	setState(t, s, "susp-1", domain.StateReview, now.Add(-time.Hour))
	if err := s.SetCardSuspended(ctx, "susp-1", &now); err != nil {
		t.Fatalf("suspend: %v", err)
	}

	items, err := s.DueQueue(ctx, "deck-1", store.QueueParams{
		Now: now, Limit: 50, NewLimit: 20, ReviewLimit: 200,
	})
	if err != nil {
		t.Fatalf("DueQueue: %v", err)
	}

	got := ids(items)
	want := []string{"learn-1", "review-1", "new-1", "new-2"}
	if len(got) != len(want) {
		t.Fatalf("queue = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("queue = %v, want %v", got, want)
		}
	}
}

func TestDueQueueRespectsNewCardLimit(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	mustDeck(t, s, "deck-1", "Deck")

	for i := 0; i < 5; i++ {
		mustCard(t, s, "deck-1", fmt.Sprintf("new-%d", i), fmt.Sprintf("w%d", i),
			baseTime.Add(time.Duration(i)*time.Minute))
	}

	items, err := s.DueQueue(ctx, "deck-1", store.QueueParams{
		Now: baseTime.Add(time.Hour), Limit: 50, NewLimit: 2, ReviewLimit: 200,
	})
	if err != nil {
		t.Fatalf("DueQueue: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("queue length = %d, want 2 (new limit)", len(items))
	}

	none, err := s.DueQueue(ctx, "deck-1", store.QueueParams{
		Now: baseTime.Add(time.Hour), Limit: 50, NewLimit: 0, ReviewLimit: 200,
	})
	if err != nil {
		t.Fatalf("DueQueue(0 new): %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("queue length = %d with NewLimit 0, want 0", len(none))
	}
}

func TestDeckCountsAndReviewTotals(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	mustDeck(t, s, "deck-1", "Deck")
	mustCard(t, s, "deck-1", "c-new", "a", baseTime)
	mustCard(t, s, "deck-1", "c-learn", "b", baseTime)
	mustCard(t, s, "deck-1", "c-future", "c", baseTime)
	mustCard(t, s, "deck-1", "c-susp", "d", baseTime)

	now := baseTime.Add(24 * time.Hour)
	setState(t, s, "c-learn", domain.StateLearning, now.Add(-time.Minute))
	setState(t, s, "c-future", domain.StateReview, now.Add(time.Hour))
	if err := s.SetCardSuspended(ctx, "c-susp", &now); err != nil {
		t.Fatalf("suspend: %v", err)
	}

	counts, err := s.DeckCounts(ctx, "deck-1", now)
	if err != nil {
		t.Fatalf("DeckCounts: %v", err)
	}
	if counts.Total != 4 {
		t.Fatalf("Total = %d, want 4", counts.Total)
	}
	if counts.Due != 2 {
		t.Fatalf("Due = %d, want 2 (c-new, c-learn)", counts.Due)
	}
	// c-susp is state New too, but suspended cards are excluded from the New,
	// Due, and Learning counts — they show up only in Total and Suspended.
	if counts.New != 1 {
		t.Fatalf("New = %d, want 1 (c-new; c-susp is new but suspended)", counts.New)
	}
	if counts.Learning != 1 {
		t.Fatalf("Learning = %d, want 1", counts.Learning)
	}
	if counts.Suspended != 1 {
		t.Fatalf("Suspended = %d, want 1", counts.Suspended)
	}

	for i, r := range []struct {
		card  string
		state domain.State
	}{
		{"c-new", domain.StateNew},
		{"c-learn", domain.StateLearning},
		{"c-learn", domain.StateLearning},
	} {
		if err := s.AppendReview(ctx, &domain.Review{
			CardID: r.card, Rating: domain.RatingGood, State: r.state,
			Due: now, ReviewedAt: now.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("AppendReview: %v", err)
		}
	}

	totals, err := s.ReviewTotals(ctx, "deck-1", now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("ReviewTotals: %v", err)
	}
	if totals.Reviews != 3 {
		t.Fatalf("Reviews = %d, want 3", totals.Reviews)
	}
	if totals.NewCards != 1 {
		t.Fatalf("NewCards = %d, want 1", totals.NewCards)
	}

	hist, err := s.ReviewHistogram(ctx, "deck-1", now.Add(-30*24*time.Hour), now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("ReviewHistogram: %v", err)
	}
	if len(hist) != 1 || hist[0].Count != 3 {
		t.Fatalf("histogram = %v, want one day with 3", hist)
	}
	if hist[0].Date != now.Format("2006-01-02") {
		t.Fatalf("histogram date = %q, want %q", hist[0].Date, now.Format("2006-01-02"))
	}
}

func TestNextDue(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	mustDeck(t, s, "deck-1", "Deck")

	now := baseTime.Add(24 * time.Hour)

	// Nothing scheduled ahead of `now` yet.
	next, err := s.NextDue(ctx, "deck-1", now)
	if err != nil {
		t.Fatalf("NextDue on empty deck: %v", err)
	}
	if next != nil {
		t.Fatalf("NextDue = %v, want nil for a deck with no cards", next)
	}

	mustCard(t, s, "deck-1", "c-soon", "a", baseTime)
	mustCard(t, s, "deck-1", "c-later", "b", baseTime)
	mustCard(t, s, "deck-1", "c-susp", "c", baseTime)

	soon := now.Add(3 * time.Hour)
	setState(t, s, "c-soon", domain.StateReview, soon)
	setState(t, s, "c-later", domain.StateReview, now.Add(48*time.Hour))
	setState(t, s, "c-susp", domain.StateReview, now.Add(time.Minute))
	if err := s.SetCardSuspended(ctx, "c-susp", &now); err != nil {
		t.Fatalf("suspend: %v", err)
	}

	next, err = s.NextDue(ctx, "deck-1", now)
	if err != nil {
		t.Fatalf("NextDue: %v", err)
	}
	if next == nil {
		t.Fatal("NextDue = nil, want the soonest unsuspended card")
	}
	if !next.Equal(soon) {
		t.Fatalf("NextDue = %v, want %v — a suspended card must not count", next, soon)
	}

	// A card due exactly now is not "next"; it is due already.
	past, err := s.NextDue(ctx, "deck-1", now.Add(72*time.Hour))
	if err != nil {
		t.Fatalf("NextDue beyond every card: %v", err)
	}
	if past != nil {
		t.Fatalf("NextDue = %v, want nil when every card is already due", past)
	}
}

func setState(t *testing.T, s *sqlite.Store, cardID string, state domain.State, due time.Time) {
	t.Helper()
	st, err := s.GetCardState(context.Background(), cardID)
	if err != nil {
		t.Fatalf("GetCardState %s: %v", cardID, err)
	}
	st.State = state
	st.Due = due
	if err := s.UpsertCardState(context.Background(), st); err != nil {
		t.Fatalf("UpsertCardState %s: %v", cardID, err)
	}
}

func ids(items []store.QueueItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Card.ID)
	}
	return out
}
