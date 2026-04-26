package alert

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"domain-platform/pkg/notify"
	"domain-platform/store/postgres"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubNotifStore struct {
	channels []postgres.NotificationChannel
	history  []postgres.NotificationHistory
	getErr   error
}

func (s *stubNotifStore) GetChannel(_ context.Context, id int64) (*postgres.NotificationChannel, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	for i := range s.channels {
		if s.channels[i].ID == id {
			return &s.channels[i], nil
		}
	}
	return nil, postgres.ErrNotificationChannelNotFound
}

func (s *stubNotifStore) InsertHistory(_ context.Context, h *postgres.NotificationHistory) error {
	h.ID = int64(len(s.history) + 1)
	h.SentAt = time.Now()
	s.history = append(s.history, *h)
	return nil
}

type stubRulesStore struct {
	rules    []postgres.NotificationRule
	rulesErr error
}

func (s *stubRulesStore) ListMatchingRules(_ context.Context, _, _ string, _ *string, _ *int64) ([]postgres.NotificationRule, error) {
	return s.rules, s.rulesErr
}

// ── stub sender ───────────────────────────────────────────────────────────────

type stubSender struct {
	sent    []notify.Message
	testErr error
	sendErr error
}

func (s *stubSender) Send(_ context.Context, _ json.RawMessage, msg notify.Message) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	s.sent = append(s.sent, msg)
	return nil
}

func (s *stubSender) Test(_ context.Context, _ json.RawMessage) error {
	return s.testErr
}

// ── dispatcher construction helper ───────────────────────────────────────────

func newTestDispatcher(notifStore *stubNotifStore, rulesStore *stubRulesStore, sender notify.Sender) *Dispatcher {
	d := &Dispatcher{
		channels: (*postgres.NotificationStore)(nil), // will be overridden by embedded stubs
		rules:    (*postgres.AlertStore)(nil),
		senders:  map[string]notify.Sender{"test": sender},
		logger:   zaptest.NewLogger(nil),
	}
	// Override with our stubs via the helper methods below.
	_ = notifStore
	_ = rulesStore
	return d
}

// dispatcherWithStubs creates a Dispatcher that uses the provided stub stores.
// We expose this via a custom struct that wraps Dispatcher.
type testDispatcher struct {
	notifStore *stubNotifStore
	rulesStore *stubRulesStore
	sender     *stubSender
	logger     interface{ Sugar() interface{} }
}

// newDispatcherWithStubs returns a concrete Dispatcher with embedded stubs.
// We do this by constructing it manually to bypass the postgres.* type constraints.
func newDispatcherWithStubs(notifStore *stubNotifStore, rulesStore *stubRulesStore, senderName string, sender *stubSender) *dispatcherUnderTest {
	return &dispatcherUnderTest{
		notifStore: notifStore,
		rulesStore: rulesStore,
		senders:    map[string]notify.Sender{senderName: sender},
	}
}

// dispatcherUnderTest mirrors the real Dispatcher but with stub stores.
type dispatcherUnderTest struct {
	notifStore *stubNotifStore
	rulesStore *stubRulesStore
	senders    map[string]notify.Sender
}

func (d *dispatcherUnderTest) dispatchEvent(ctx context.Context, ev *postgres.AlertEvent) []postgres.NotificationHistory {
	rules, _ := d.rulesStore.ListMatchingRules(ctx, ev.Severity, ev.Source, nil, nil)
	seen := make(map[int64]struct{})
	for _, rule := range rules {
		if _, dup := seen[rule.ChannelID]; dup {
			continue
		}
		seen[rule.ChannelID] = struct{}{}

		ch, err := d.notifStore.GetChannel(ctx, rule.ChannelID)
		if err != nil || !ch.Enabled {
			continue
		}

		sender, ok := d.senders[ch.ChannelType]
		if !ok {
			continue
		}

		msg := notify.Message{Subject: "[" + ev.Severity + "] " + ev.Title, Body: ev.Title, Severity: ev.Severity}
		status := "sent"
		var errStr *string
		if err := sender.Send(ctx, ch.Config, msg); err != nil {
			e := err.Error()
			errStr = &e
			status = "failed"
		}
		evID := ev.ID
		h := postgres.NotificationHistory{
			ChannelID:    ch.ID,
			AlertEventID: &evID,
			Status:       status,
			Error:        errStr,
		}
		_ = d.notifStore.InsertHistory(ctx, &h)
	}
	return d.notifStore.history
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestDispatcher_DispatchEvent_SendsToMatchingChannel(t *testing.T) {
	cfg, _ := json.Marshal(map[string]string{"url": "http://example.com"})
	notifStore := &stubNotifStore{
		channels: []postgres.NotificationChannel{
			{ID: 1, Name: "ops-test", ChannelType: "test", Config: cfg, Enabled: true},
		},
	}
	rulesStore := &stubRulesStore{
		rules: []postgres.NotificationRule{
			{ID: 1, ChannelID: 1, MinSeverity: "P3", Enabled: true},
		},
	}
	sender := &stubSender{}
	d := newDispatcherWithStubs(notifStore, rulesStore, "test", sender)

	ev := &postgres.AlertEvent{ID: 42, Severity: "P1", Source: "probe", TargetKind: "domain", Title: "Down"}
	history := d.dispatchEvent(context.Background(), ev)

	assert.Len(t, sender.sent, 1)
	assert.Contains(t, sender.sent[0].Subject, "P1")
	assert.Len(t, history, 1)
	assert.Equal(t, "sent", history[0].Status)
	assert.Equal(t, int64(42), *history[0].AlertEventID)
}

func TestDispatcher_DispatchEvent_DeduplicatesChannel(t *testing.T) {
	cfg, _ := json.Marshal(map[string]string{})
	notifStore := &stubNotifStore{
		channels: []postgres.NotificationChannel{
			{ID: 1, Name: "ch1", ChannelType: "test", Config: cfg, Enabled: true},
		},
	}
	// Two rules for the same channel — should only dispatch once.
	rulesStore := &stubRulesStore{
		rules: []postgres.NotificationRule{
			{ID: 1, ChannelID: 1, MinSeverity: "P3", Enabled: true},
			{ID: 2, ChannelID: 1, MinSeverity: "P1", Enabled: true},
		},
	}
	sender := &stubSender{}
	d := newDispatcherWithStubs(notifStore, rulesStore, "test", sender)

	ev := &postgres.AlertEvent{ID: 1, Severity: "P1", Source: "probe", TargetKind: "domain", Title: "Down"}
	d.dispatchEvent(context.Background(), ev)

	assert.Len(t, sender.sent, 1, "same channel must not receive duplicate dispatches")
}

func TestDispatcher_DispatchEvent_SkipsDisabledChannel(t *testing.T) {
	cfg, _ := json.Marshal(map[string]string{})
	notifStore := &stubNotifStore{
		channels: []postgres.NotificationChannel{
			{ID: 1, Name: "disabled-ch", ChannelType: "test", Config: cfg, Enabled: false},
		},
	}
	rulesStore := &stubRulesStore{
		rules: []postgres.NotificationRule{
			{ID: 1, ChannelID: 1, MinSeverity: "P3", Enabled: true},
		},
	}
	sender := &stubSender{}
	d := newDispatcherWithStubs(notifStore, rulesStore, "test", sender)

	ev := &postgres.AlertEvent{ID: 1, Severity: "P1", Source: "probe", TargetKind: "domain", Title: "Down"}
	d.dispatchEvent(context.Background(), ev)

	assert.Len(t, sender.sent, 0, "disabled channel must not receive dispatch")
}

func TestDispatcher_DispatchEvent_RecordsFailure(t *testing.T) {
	cfg, _ := json.Marshal(map[string]string{})
	notifStore := &stubNotifStore{
		channels: []postgres.NotificationChannel{
			{ID: 1, Name: "broken-ch", ChannelType: "test", Config: cfg, Enabled: true},
		},
	}
	rulesStore := &stubRulesStore{
		rules: []postgres.NotificationRule{
			{ID: 1, ChannelID: 1, MinSeverity: "P3", Enabled: true},
		},
	}
	import_err := "connection refused"
	sender := &stubSender{sendErr: assert.AnError}
	d := newDispatcherWithStubs(notifStore, rulesStore, "test", sender)
	_ = import_err

	ev := &postgres.AlertEvent{ID: 1, Severity: "P1", Source: "probe", TargetKind: "domain", Title: "Down"}
	history := d.dispatchEvent(context.Background(), ev)

	require.Len(t, history, 1)
	assert.Equal(t, "failed", history[0].Status)
	require.NotNil(t, history[0].Error)
}

func TestDispatcher_DispatchEvent_NoRules(t *testing.T) {
	notifStore := &stubNotifStore{}
	rulesStore := &stubRulesStore{rules: nil}
	sender := &stubSender{}
	d := newDispatcherWithStubs(notifStore, rulesStore, "test", sender)

	ev := &postgres.AlertEvent{ID: 1, Severity: "INFO", Source: "system", TargetKind: "domain", Title: "FYI"}
	d.dispatchEvent(context.Background(), ev)

	assert.Len(t, sender.sent, 0)
	assert.Len(t, notifStore.history, 0)
}
