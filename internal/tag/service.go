package tag

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"go.uber.org/zap"

	"domain-platform/store/postgres"
)

var (
	ErrNotFound     = errors.New("tag not found")
	ErrDuplicateName = errors.New("tag name already exists")
	ErrEmptyName    = errors.New("tag name is required")
	ErrInvalidColor = errors.New("invalid hex color (expected #RRGGBB)")
)

// colorRe matches #RRGGBB hex colours.
var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ValidateColor checks a hex colour string. Nil is accepted (no colour).
func ValidateColor(c *string) error {
	if c == nil || *c == "" {
		return nil
	}
	if !colorRe.MatchString(*c) {
		return ErrInvalidColor
	}
	return nil
}

// Service wraps TagStore with business logic.
type Service struct {
	store       *postgres.TagStore
	domainStore *postgres.DomainStore
	logger      *zap.Logger
}

func NewService(store *postgres.TagStore, domainStore *postgres.DomainStore, logger *zap.Logger) *Service {
	return &Service{store: store, domainStore: domainStore, logger: logger}
}

// ── Tag CRUD ──────────────────────────────────────────────────────────────────

type CreateInput struct {
	Name  string
	Color *string
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*postgres.Tag, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, ErrEmptyName
	}
	if err := ValidateColor(in.Color); err != nil {
		return nil, err
	}

	t := &postgres.Tag{Name: name, Color: in.Color}
	created, err := s.store.Create(ctx, t)
	if err != nil {
		if isDuplicate(err) {
			return nil, ErrDuplicateName
		}
		return nil, fmt.Errorf("create tag: %w", err)
	}
	s.logger.Info("tag created", zap.Int64("id", created.ID), zap.String("name", name))
	return created, nil
}

func (s *Service) GetByID(ctx context.Context, id int64) (*postgres.Tag, error) {
	t, err := s.store.GetByID(ctx, id)
	if errors.Is(err, postgres.ErrTagNotFound) {
		return nil, ErrNotFound
	}
	return t, err
}

func (s *Service) ListWithCounts(ctx context.Context) ([]postgres.TagWithCount, error) {
	return s.store.ListWithCounts(ctx)
}

func (s *Service) Update(ctx context.Context, id int64, name string, color *string) (*postgres.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrEmptyName
	}
	if err := ValidateColor(color); err != nil {
		return nil, err
	}

	t, err := s.store.GetByID(ctx, id)
	if errors.Is(err, postgres.ErrTagNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Name = name
	t.Color = color
	if err := s.store.Update(ctx, t); err != nil {
		if isDuplicate(err) {
			return nil, ErrDuplicateName
		}
		return nil, fmt.Errorf("update tag: %w", err)
	}
	return t, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if _, err := s.store.GetByID(ctx, id); err != nil {
		if errors.Is(err, postgres.ErrTagNotFound) {
			return ErrNotFound
		}
		return err
	}
	// CASCADE takes care of domain_tags
	return s.store.Delete(ctx, id)
}

// ── Domain-Tag operations ─────────────────────────────────────────────────────

func (s *Service) GetDomainTags(ctx context.Context, domainID int64) ([]postgres.Tag, error) {
	return s.store.GetDomainTags(ctx, domainID)
}

func (s *Service) SetDomainTags(ctx context.Context, domainID int64, tagIDs []int64) error {
	return s.store.SetDomainTags(ctx, domainID, tagIDs)
}

// BulkAddTags adds tagIDs to every domain in domainIDs without removing existing tags.
// It first reads existing tags, then sets the union.
func (s *Service) BulkAddTags(ctx context.Context, domainIDs []int64, tagIDs []int64) error {
	for _, domainID := range domainIDs {
		existing, err := s.store.GetDomainTags(ctx, domainID)
		if err != nil {
			return fmt.Errorf("get tags for domain %d: %w", domainID, err)
		}
		set := make(map[int64]bool)
		for _, t := range existing {
			set[t.ID] = true
		}
		for _, tid := range tagIDs {
			set[tid] = true
		}
		merged := make([]int64, 0, len(set))
		for id := range set {
			merged = append(merged, id)
		}
		if err := s.store.SetDomainTags(ctx, domainID, merged); err != nil {
			return fmt.Errorf("set tags for domain %d: %w", domainID, err)
		}
	}
	return nil
}

// BulkRemoveTags removes tagIDs from every domain in domainIDs.
func (s *Service) BulkRemoveTags(ctx context.Context, domainIDs []int64, tagIDs []int64) error {
	remove := make(map[int64]bool)
	for _, tid := range tagIDs {
		remove[tid] = true
	}
	for _, domainID := range domainIDs {
		existing, err := s.store.GetDomainTags(ctx, domainID)
		if err != nil {
			return fmt.Errorf("get tags for domain %d: %w", domainID, err)
		}
		remaining := make([]int64, 0, len(existing))
		for _, t := range existing {
			if !remove[t.ID] {
				remaining = append(remaining, t.ID)
			}
		}
		if err := s.store.SetDomainTags(ctx, domainID, remaining); err != nil {
			return fmt.Errorf("set tags for domain %d: %w", domainID, err)
		}
	}
	return nil
}

// ── Bulk domain field update ──────────────────────────────────────────────────

type BulkUpdateInput struct {
	DomainIDs          []int64
	RegistrarAccountID *int64
	DNSProviderID      *int64
	AutoRenew          *bool
}

func (s *Service) BulkUpdateFields(ctx context.Context, in BulkUpdateInput) (int64, error) {
	if len(in.DomainIDs) == 0 {
		return 0, nil
	}
	return s.domainStore.BulkUpdateFields(ctx, in.DomainIDs, in.RegistrarAccountID, in.DNSProviderID, in.AutoRenew)
}

// ── Export ─────────────────────────────────────────────────────────────────────

// ExportDomains returns all domains matching the filter for CSV export.
func (s *Service) ExportDomains(ctx context.Context, f postgres.ListFilter) ([]postgres.Domain, error) {
	return s.domainStore.ListWithFilter(ctx, f)
}

// ExportDomainsEnriched returns enriched domain rows (with registrar/CDN names)
// for CSV export. The filter limit is set to 10 000 to bound output size.
func (s *Service) ExportDomainsEnriched(ctx context.Context, f postgres.ListFilter) ([]postgres.DomainListRow, error) {
	f.Limit = 10000
	return s.domainStore.ListEnriched(ctx, f)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func isDuplicate(err error) bool {
	return strings.Contains(err.Error(), "uq_tags_name") || strings.Contains(err.Error(), "duplicate key")
}
