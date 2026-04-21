package cost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"domain-platform/store/postgres"
)

var (
	ErrFeeScheduleNotFound = errors.New("fee schedule not found")
	ErrCostNotFound        = errors.New("domain cost not found")
	ErrInvalidCostType     = errors.New("invalid cost type")
	ErrInvalidCurrency     = errors.New("invalid currency code")
)

// ValidCostTypes enumerates allowed values for domain_costs.cost_type.
var ValidCostTypes = map[string]bool{
	"registration": true,
	"renewal":      true,
	"transfer":     true,
	"privacy":      true,
	"other":        true,
}

// ValidCurrencies enumerates commonly accepted ISO-4217 codes.
var ValidCurrencies = map[string]bool{
	"USD": true, "EUR": true, "GBP": true, "CNY": true,
	"TWD": true, "JPY": true, "AUD": true, "CAD": true,
	"HKD": true, "SGD": true, "KRW": true, "INR": true,
}

// Service wraps CostStore, DomainStore, and RegistrarStore.
type Service struct {
	store       *postgres.CostStore
	domainStore *postgres.DomainStore
	regStore    *postgres.RegistrarStore
	logger      *zap.Logger
}

func NewService(
	store *postgres.CostStore,
	domainStore *postgres.DomainStore,
	regStore *postgres.RegistrarStore,
	logger *zap.Logger,
) *Service {
	return &Service{
		store:       store,
		domainStore: domainStore,
		regStore:    regStore,
		logger:      logger,
	}
}

// ── Fee Schedules ─────────────────────────────────────────────────────────────

type CreateFeeScheduleInput struct {
	RegistrarID     int64
	TLD             string
	RegistrationFee float64
	RenewalFee      float64
	TransferFee     float64
	PrivacyFee      float64
	Currency        string
}

type UpdateFeeScheduleInput struct {
	RegistrationFee float64
	RenewalFee      float64
	TransferFee     float64
	PrivacyFee      float64
	Currency        string
}

func (s *Service) CreateFeeSchedule(ctx context.Context, in CreateFeeScheduleInput) (*postgres.DomainFeeSchedule, error) {
	if err := validateCurrency(in.Currency); err != nil {
		return nil, err
	}
	tld := normalizeTLD(in.TLD)
	if tld == "" {
		return nil, fmt.Errorf("tld is required")
	}

	fs := &postgres.DomainFeeSchedule{
		RegistrarID:     in.RegistrarID,
		TLD:             tld,
		RegistrationFee: in.RegistrationFee,
		RenewalFee:      in.RenewalFee,
		TransferFee:     in.TransferFee,
		PrivacyFee:      in.PrivacyFee,
		Currency:        in.Currency,
	}
	created, err := s.store.CreateFeeSchedule(ctx, fs)
	if err != nil {
		if isDuplicateTLD(err) {
			return nil, fmt.Errorf("fee schedule for registrar %d + TLD %s already exists", in.RegistrarID, tld)
		}
		return nil, fmt.Errorf("create fee schedule: %w", err)
	}
	s.logger.Info("fee schedule created",
		zap.Int64("id", created.ID),
		zap.Int64("registrar_id", in.RegistrarID),
		zap.String("tld", tld),
	)
	return created, nil
}

func (s *Service) GetFeeScheduleByID(ctx context.Context, id int64) (*postgres.DomainFeeSchedule, error) {
	fs, err := s.store.GetFeeScheduleByID(ctx, id)
	if errors.Is(err, postgres.ErrFeeScheduleNotFound) {
		return nil, ErrFeeScheduleNotFound
	}
	return fs, err
}

func (s *Service) ListFeeSchedules(ctx context.Context, registrarID *int64) ([]postgres.DomainFeeSchedule, error) {
	return s.store.ListFeeSchedules(ctx, registrarID)
}

func (s *Service) UpdateFeeSchedule(ctx context.Context, id int64, in UpdateFeeScheduleInput) (*postgres.DomainFeeSchedule, error) {
	fs, err := s.store.GetFeeScheduleByID(ctx, id)
	if errors.Is(err, postgres.ErrFeeScheduleNotFound) {
		return nil, ErrFeeScheduleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get fee schedule: %w", err)
	}
	if err := validateCurrency(in.Currency); err != nil {
		return nil, err
	}

	fs.RegistrationFee = in.RegistrationFee
	fs.RenewalFee = in.RenewalFee
	fs.TransferFee = in.TransferFee
	fs.PrivacyFee = in.PrivacyFee
	fs.Currency = in.Currency

	if err := s.store.UpdateFeeSchedule(ctx, fs); err != nil {
		return nil, fmt.Errorf("update fee schedule: %w", err)
	}
	s.logger.Info("fee schedule updated", zap.Int64("id", id))
	return fs, nil
}

func (s *Service) DeleteFeeSchedule(ctx context.Context, id int64) error {
	if _, err := s.store.GetFeeScheduleByID(ctx, id); err != nil {
		if errors.Is(err, postgres.ErrFeeScheduleNotFound) {
			return ErrFeeScheduleNotFound
		}
		return err
	}
	return s.store.DeleteFeeSchedule(ctx, id)
}

// ── Cost Records ──────────────────────────────────────────────────────────────

type CreateCostInput struct {
	DomainID    int64
	CostType    string
	Amount      float64
	Currency    string
	PeriodStart *time.Time
	PeriodEnd   *time.Time
	PaidAt      *time.Time
	Notes       *string
}

func (s *Service) CreateCost(ctx context.Context, in CreateCostInput) (*postgres.DomainCost, error) {
	if !ValidCostTypes[in.CostType] {
		return nil, ErrInvalidCostType
	}
	if err := validateCurrency(in.Currency); err != nil {
		return nil, err
	}

	c := &postgres.DomainCost{
		DomainID:    in.DomainID,
		CostType:    in.CostType,
		Amount:      in.Amount,
		Currency:    in.Currency,
		PeriodStart: in.PeriodStart,
		PeriodEnd:   in.PeriodEnd,
		PaidAt:      in.PaidAt,
		Notes:       in.Notes,
	}
	created, err := s.store.CreateCost(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("create domain cost: %w", err)
	}
	return created, nil
}

func (s *Service) ListCostsByDomain(ctx context.Context, domainID int64) ([]postgres.DomainCost, error) {
	return s.store.ListCostsByDomain(ctx, domainID)
}

// ── Auto-calculation ──────────────────────────────────────────────────────────

// TryApplyFeeSchedule looks up the fee schedule for a domain's (registrar, tld)
// combination and sets annual_cost if the domain has fee_fixed = false.
// Returns nil (no-op) if the domain has no registrar account, no TLD, or no
// matching fee schedule — we never fail a domain operation because of a missing price.
func (s *Service) TryApplyFeeSchedule(ctx context.Context, domainID int64) error {
	domain, err := s.domainStore.GetByID(ctx, domainID)
	if err != nil {
		return fmt.Errorf("get domain: %w", err)
	}
	if domain.FeeFixed {
		return nil // operator set a fixed price — honour it
	}
	if domain.RegistrarAccountID == nil || domain.TLD == nil {
		return nil // not enough info to look up fee schedule
	}

	account, err := s.regStore.GetAccountByID(ctx, *domain.RegistrarAccountID)
	if err != nil {
		return nil // account not found or deleted — skip silently
	}

	fs, err := s.store.GetFeeSchedule(ctx, account.RegistrarID, *domain.TLD)
	if errors.Is(err, postgres.ErrFeeScheduleNotFound) {
		return nil // no schedule defined for this registrar+TLD — keep current cost
	}
	if err != nil {
		return fmt.Errorf("get fee schedule: %w", err)
	}

	if err := s.domainStore.UpdateAnnualCost(ctx, domainID, fs.RenewalFee, fs.Currency); err != nil {
		return fmt.Errorf("apply fee schedule: %w", err)
	}
	s.logger.Info("annual cost applied from fee schedule",
		zap.Int64("domain_id", domainID),
		zap.Float64("renewal_fee", fs.RenewalFee),
		zap.String("currency", fs.Currency),
	)
	return nil
}

// RecalculateAllCosts iterates every non-fee-fixed domain and applies the fee
// schedule. Returns counts of (updated, skipped, failed).
func (s *Service) RecalculateAllCosts(ctx context.Context) (updated, skipped, failed int) {
	feeFixed := false
	_ = feeFixed // used to conceptually represent the filter
	domains, err := s.domainStore.ListWithFilter(ctx, postgres.ListFilter{Limit: 10000})
	if err != nil {
		s.logger.Error("recalculate costs: list domains", zap.Error(err))
		return
	}
	for _, d := range domains {
		if d.FeeFixed {
			skipped++
			continue
		}
		if err := s.TryApplyFeeSchedule(ctx, d.ID); err != nil {
			s.logger.Warn("recalculate costs: apply failed",
				zap.Int64("domain_id", d.ID), zap.Error(err))
			failed++
		} else {
			updated++
		}
	}
	return
}

// ── Aggregates ────────────────────────────────────────────────────────────────

// GetCostSummary returns aggregated annual_cost totals. groupBy must be
// "registrar" or "tld".
func (s *Service) GetCostSummary(ctx context.Context, groupBy string) ([]postgres.CostSummary, error) {
	switch groupBy {
	case "registrar":
		return s.store.GetCostSummaryByRegistrar(ctx)
	case "tld":
		return s.store.GetCostSummaryByTLD(ctx)
	default:
		return nil, fmt.Errorf("invalid group_by: must be 'registrar' or 'tld'")
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func normalizeTLD(tld string) string {
	tld = strings.ToLower(strings.TrimSpace(tld))
	if tld != "" && !strings.HasPrefix(tld, ".") {
		tld = "." + tld
	}
	return tld
}

func validateCurrency(code string) error {
	if !ValidCurrencies[strings.ToUpper(code)] {
		return fmt.Errorf("%w: %q", ErrInvalidCurrency, code)
	}
	return nil
}

func isDuplicateTLD(err error) bool {
	return strings.Contains(err.Error(), "uq_fee_schedules_registrar_tld") ||
		strings.Contains(err.Error(), "duplicate key")
}
