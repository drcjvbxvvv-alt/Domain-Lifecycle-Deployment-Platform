package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"domain-platform/internal/tasks"
	"domain-platform/store/postgres"
)

// Service orchestrates probe scheduling and result persistence.
// It is the single write path for probe_tasks and probe_results.
type Service struct {
	probeStore  *postgres.ProbeStore
	domainStore *postgres.DomainStore
	asynqClient *asynq.Client
	logger      *zap.Logger
}

func NewService(
	probeStore *postgres.ProbeStore,
	domainStore *postgres.DomainStore,
	asynqClient *asynq.Client,
	logger *zap.Logger,
) *Service {
	return &Service{
		probeStore:  probeStore,
		domainStore: domainStore,
		asynqClient: asynqClient,
		logger:      logger,
	}
}

// ScheduleAll creates probe_task rows for all enabled policies and enqueues
// the corresponding asynq tasks. It is called by the periodic batch handler.
func (s *Service) ScheduleAll(ctx context.Context) error {
	policies, err := s.probeStore.ListPolicies(ctx, nil, true /* enabledOnly */)
	if err != nil {
		return fmt.Errorf("schedule_all: list policies: %w", err)
	}
	if len(policies) == 0 {
		s.logger.Info("probe schedule_all: no enabled policies")
		return nil
	}

	domains, err := s.domainStore.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("schedule_all: list active domains: %w", err)
	}
	if len(domains) == 0 {
		return nil
	}

	enqueued := 0
	for _, policy := range policies {
		taskType := taskTypeForTier(policy.Tier)
		if taskType == "" {
			continue
		}
		for _, d := range domains {
			// Skip domains that don't match the policy's project filter.
			if policy.ProjectID != nil && d.ProjectID != *policy.ProjectID {
				continue
			}
			task, err := s.createAndEnqueue(ctx, &policy, &d, taskType)
			if err != nil {
				s.logger.Warn("probe schedule_all: enqueue failed",
					zap.Int64("domain_id", d.ID),
					zap.Int64("policy_id", policy.ID),
					zap.Error(err),
				)
				continue
			}
			enqueued++
			_ = task
		}
	}

	s.logger.Info("probe schedule_all done",
		zap.Int("policies", len(policies)),
		zap.Int("domains", len(domains)),
		zap.Int("enqueued", enqueued),
	)
	return nil
}

// createAndEnqueue inserts a probe_task row and enqueues the asynq task atomically.
func (s *Service) createAndEnqueue(
	ctx context.Context,
	policy *postgres.ProbePolicy,
	domain *postgres.Domain,
	taskType string,
) (*asynq.Task, error) {
	probeTask := &postgres.ProbeTask{
		PolicyID:     policy.ID,
		DomainID:     domain.ID,
		ScheduledFor: time.Now(),
	}
	if err := s.probeStore.CreateTask(ctx, probeTask); err != nil {
		return nil, fmt.Errorf("create probe_task: %w", err)
	}

	payload := RunPayload{
		ProbeTaskID: probeTask.ID,
		PolicyID:    policy.ID,
		DomainID:    domain.ID,
		FQDN:        domain.FQDN,
	}
	if policy.ExpectedStatus != nil {
		v := *policy.ExpectedStatus
		payload.ExpectedStatus = &v
	}
	payload.ExpectedKeyword = policy.ExpectedKeyword
	payload.ExpectedMetaTag = policy.ExpectedMetaTag
	payload.TimeoutSeconds = policy.TimeoutSeconds

	raw, _ := json.Marshal(payload)
	task := asynq.NewTask(taskType, raw,
		asynq.MaxRetry(2),
		asynq.Timeout(time.Duration(policy.TimeoutSeconds+5)*time.Second),
		asynq.Queue("probe"),
	)
	if _, err := s.asynqClient.EnqueueContext(ctx, task); err != nil {
		return nil, fmt.Errorf("enqueue asynq task: %w", err)
	}
	return task, nil
}

// SaveResult persists a CheckResult to probe_results.
func (s *Service) SaveResult(ctx context.Context, taskID, domainID, policyID int64, tier int16, r CheckResult) error {
	detailJSON, _ := json.Marshal(r.Detail)

	row := &postgres.ProbeResult{
		DomainID:       domainID,
		PolicyID:       &policyID,
		ProbeTaskID:    &taskID,
		Tier:           tier,
		Status:         r.Status,
		HTTPStatus:     r.HTTPStatus,
		ResponseTimeMS: r.ResponseTimeMS,
		ResponseSizeB:  r.ResponseSizeB,
		TLSHandshakeOK: r.TLSHandshakeOK,
		CertExpiresAt:  r.CertExpiresAt,
		ContentHash:    r.ContentHash,
		ErrorMessage:   r.ErrorMessage,
		Detail:         detailJSON,
		CheckedAt:      time.Now(),
	}
	runner := "worker"
	row.ProbeRunner = &runner

	return s.probeStore.SaveResult(ctx, row)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func taskTypeForTier(tier int16) string {
	switch tier {
	case 1:
		return tasks.TypeProbeRunL1
	case 2:
		return tasks.TypeProbeRunL2
	case 3:
		return tasks.TypeProbeRunL3
	}
	return ""
}

// RunPayload is the asynq task payload for all three probe tiers.
type RunPayload struct {
	ProbeTaskID     int64   `json:"probe_task_id"`
	PolicyID        int64   `json:"policy_id"`
	DomainID        int64   `json:"domain_id"`
	FQDN            string  `json:"fqdn"`
	ExpectedStatus  *int    `json:"expected_status,omitempty"`
	ExpectedKeyword *string `json:"expected_keyword,omitempty"`
	ExpectedMetaTag *string `json:"expected_meta_tag,omitempty"`
	TimeoutSeconds  int     `json:"timeout_seconds"`
	HealthPath      string  `json:"health_path,omitempty"` // L3 only
}
