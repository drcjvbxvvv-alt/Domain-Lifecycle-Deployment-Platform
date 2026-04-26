package alert

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"domain-platform/store/postgres"
)

// ── stub alert store ──────────────────────────────────────────────────────────

type stubAlertStore struct {
	events      []*postgres.AlertEvent
	rules       []postgres.NotificationRule
	dedupExists bool
	insertErr   error
	rulesErr    error
}

func (s *stubAlertStore) ExistsActiveDedupKey(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return s.dedupExists, nil
}
func (s *stubAlertStore) Insert(_ context.Context, ev *postgres.AlertEvent) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	ev.ID = int64(len(s.events) + 1)
	ev.UUID = "test-uuid"
	ev.CreatedAt = time.Now()
	s.events = append(s.events, ev)
	return nil
}
func (s *stubAlertStore) ListMatchingRules(_ context.Context, _, _ string, _ *string, _ *int64) ([]postgres.NotificationRule, error) {
	return s.rules, s.rulesErr
}
func (s *stubAlertStore) MarkNotified(_ context.Context, _ int64) error { return nil }
func (s *stubAlertStore) GetByID(_ context.Context, id int64) (*postgres.AlertEvent, error) {
	for _, ev := range s.events {
		if ev.ID == id {
			return ev, nil
		}
	}
	return nil, postgres.ErrAlertNotFound
}

// ── stub payload ─────────────────────────────────────────────────────────────

type stubNotifySendPayload struct {
	ChannelID    int64
	AlertEventID int64
	Severity     string
}

// ── stub engine ──────────────────────────────────────────────────────────────

// testEngine wraps the alert.Engine logic with injected stub store +
// captures what would have been enqueued to TypeNotifySend.
type testEngine struct {
	store    *stubAlertStore
	enqueued []stubNotifySendPayload
}

// Fire mirrors the real Engine.Fire logic, substituting asynq with local capture.
func (te *testEngine) Fire(ctx context.Context, ev *postgres.AlertEvent) error {
	if ev.DedupKey != nil && *ev.DedupKey != "" {
		exists, _ := te.store.ExistsActiveDedupKey(ctx, *ev.DedupKey, dedupWindow)
		if exists {
			return nil
		}
	}
	if err := te.store.Insert(ctx, ev); err != nil {
		return err
	}

	targetKind := ev.TargetKind
	rules, _ := te.store.ListMatchingRules(ctx, ev.Severity, ev.Source, &targetKind, ev.TargetID)

	seen := make(map[int64]struct{}, len(rules))
	for _, r := range rules {
		if _, dup := seen[r.ChannelID]; dup {
			continue
		}
		seen[r.ChannelID] = struct{}{}
		te.enqueued = append(te.enqueued, stubNotifySendPayload{
			ChannelID:    r.ChannelID,
			AlertEventID: ev.ID,
			Severity:     severityToLevel(ev.Severity),
		})
	}
	if len(te.enqueued) > 0 {
		_ = te.store.MarkNotified(ctx, ev.ID)
	}
	return nil
}

// ── tests ──────────────────────────────────────────────────────────────────

func TestEngine_Fire_Persists(t *testing.T) {
	store := &stubAlertStore{}
	te := &testEngine{store: store}

	ev := &postgres.AlertEvent{
		Severity:   "P1",
		Source:     "probe",
		TargetKind: "domain",
		Title:      "L1 probe failed: example.com",
	}
	err := te.Fire(context.Background(), ev)
	require.NoError(t, err)
	assert.Len(t, store.events, 1)
	assert.Equal(t, "P1", store.events[0].Severity)
}

func TestEngine_Fire_DedupSuppresses(t *testing.T) {
	store := &stubAlertStore{dedupExists: true}
	te := &testEngine{store: store}

	dk := "probe:l1:domain:42"
	ev := &postgres.AlertEvent{
		Severity:   "P1",
		Source:     "probe",
		TargetKind: "domain",
		Title:      "L1 probe failed: example.com",
		DedupKey:   &dk,
	}
	err := te.Fire(context.Background(), ev)
	require.NoError(t, err)
	// Dedup hit — event must NOT be inserted.
	assert.Len(t, store.events, 0)
}

func TestEngine_Fire_DedupAllowsNewAlert(t *testing.T) {
	store := &stubAlertStore{dedupExists: false}
	te := &testEngine{store: store}

	dk := "probe:l1:domain:42"
	ev := &postgres.AlertEvent{
		Severity:   "P1",
		Source:     "probe",
		TargetKind: "domain",
		Title:      "L1 probe failed: example.com",
		DedupKey:   &dk,
	}
	err := te.Fire(context.Background(), ev)
	require.NoError(t, err)
	assert.Len(t, store.events, 1)
}

func TestEngine_Fire_FansOutToMatchingRules(t *testing.T) {
	store := &stubAlertStore{
		rules: []postgres.NotificationRule{
			{ID: 1, ChannelID: 10, MinSeverity: "P3", Enabled: true},
			{ID: 2, ChannelID: 20, MinSeverity: "P2", Enabled: true},
		},
	}
	te := &testEngine{store: store}

	ev := &postgres.AlertEvent{
		Severity:   "P2",
		Source:     "drift",
		TargetKind: "domain",
		Title:      "DNS drift detected",
	}
	err := te.Fire(context.Background(), ev)
	require.NoError(t, err)
	assert.Len(t, te.enqueued, 2)
	assert.Equal(t, int64(10), te.enqueued[0].ChannelID)
	assert.Equal(t, int64(20), te.enqueued[1].ChannelID)
	assert.Equal(t, "error", te.enqueued[0].Severity) // P2 → "error"
}

func TestEngine_Fire_DeduplicatesChannels(t *testing.T) {
	// Two rules pointing to the same channel — should only dispatch once.
	store := &stubAlertStore{
		rules: []postgres.NotificationRule{
			{ID: 1, ChannelID: 10, MinSeverity: "P3", Enabled: true},
			{ID: 2, ChannelID: 10, MinSeverity: "P1", Enabled: true},
		},
	}
	te := &testEngine{store: store}

	ev := &postgres.AlertEvent{
		Severity:   "P1",
		Source:     "probe",
		TargetKind: "domain",
		Title:      "Host down",
	}
	err := te.Fire(context.Background(), ev)
	require.NoError(t, err)
	assert.Len(t, te.enqueued, 1, "should deduplicate same channel")
}

func TestEngine_Fire_NoRules(t *testing.T) {
	store := &stubAlertStore{rules: nil}
	te := &testEngine{store: store}

	ev := &postgres.AlertEvent{
		Severity:   "INFO",
		Source:     "system",
		TargetKind: "domain",
		Title:      "Informational event",
	}
	err := te.Fire(context.Background(), ev)
	require.NoError(t, err)
	assert.Len(t, te.enqueued, 0)
	assert.Len(t, store.events, 1) // event is still persisted even without rules
}

func TestSeverityToLevel(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"P1", "critical"},
		{"P2", "error"},
		{"P3", "warning"},
		{"INFO", "info"},
		{"unknown", "info"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, severityToLevel(tt.in), "input: %s", tt.in)
	}
}
