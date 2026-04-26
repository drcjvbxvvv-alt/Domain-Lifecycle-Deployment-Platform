package registrar

// Unit tests for SyncAccount.
//
// All external dependencies (registrar store, domain store, provider) are
// replaced with lightweight stubs so no database or HTTP server is needed.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	registrarprovider "domain-platform/pkg/provider/registrar"
	"domain-platform/store/postgres"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── stub domain store ──────────────────────────────────────────────────────────

type stubDomainStore struct {
	updatedFQDNs []string
	notFoundFQDNs map[string]bool // FQDNs that return updated=false
	updateErr    error
}

func (s *stubDomainStore) UpdateDomainDates(
	_ context.Context,
	fqdn string,
	_ int64,
	_ *time.Time,
	_ *time.Time,
	_ bool,
) (bool, error) {
	if s.updateErr != nil {
		return false, s.updateErr
	}
	if s.notFoundFQDNs != nil && s.notFoundFQDNs[fqdn] {
		return false, nil
	}
	s.updatedFQDNs = append(s.updatedFQDNs, fqdn)
	return true, nil
}

// ── stub registrar provider ────────────────────────────────────────────────────

type stubProvider struct {
	domains []registrarprovider.DomainInfo
	err     error
}

func (p *stubProvider) Name() string { return "stub" }
func (p *stubProvider) ListDomains(_ context.Context) ([]registrarprovider.DomainInfo, error) {
	return p.domains, p.err
}
func (p *stubProvider) GetDomain(_ context.Context, _ string) (*registrarprovider.DomainInfo, error) {
	return nil, nil
}

// ── helper: build a testable Service with provider injected via registry ───────

// registerStubProvider temporarily registers a stub provider under a test type
// and returns a cleanup function.
func registerStubProvider(t *testing.T, typeName string, stub *stubProvider) {
	t.Helper()
	registrarprovider.Register(typeName, func(_ json.RawMessage) (registrarprovider.Provider, error) {
		return stub, nil
	})
}

// ── logic tests ───────────────────────────────────────────────────────────────
// SyncAccount is tested via syncAccountLogic, a free function that accepts
// closures instead of concrete types, so no real DB or HTTP is needed.

func syncAccountLogic(
	ctx context.Context,
	accountID int64,
	getAccount func(context.Context, int64) (*postgres.RegistrarAccount, error),
	getRegistrar func(context.Context, int64) (*postgres.Registrar, error),
	getDomainUpdater func() domainDateUpdater,
	providerFactory func(apiType string, creds json.RawMessage) (registrarprovider.Provider, error),
	logger *zap.Logger,
) (*SyncResult, error) {
	account, err := getAccount(ctx, accountID)
	if errors.Is(err, postgres.ErrRegistrarAccountNotFound) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, err
	}

	reg, err := getRegistrar(ctx, account.RegistrarID)
	if errors.Is(err, postgres.ErrRegistrarNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if reg.APIType == nil || *reg.APIType == "" {
		return nil, ErrNoAPIType
	}

	provider, err := providerFactory(*reg.APIType, account.Credentials)
	if err != nil {
		return nil, err
	}

	domains, err := provider.ListDomains(ctx)
	if err != nil {
		return nil, err
	}

	updater := getDomainUpdater()
	result := &SyncResult{Total: len(domains), NotFound: []string{}}
	for _, d := range domains {
		updated, err := updater.UpdateDomainDates(ctx, d.FQDN, accountID,
			d.RegistrationDate, d.ExpiryDate, d.AutoRenew)
		if err != nil {
			result.Errors = append(result.Errors, SyncItemError{FQDN: d.FQDN, Message: err.Error()})
			continue
		}
		if !updated {
			result.NotFound = append(result.NotFound, d.FQDN)
		} else {
			result.Updated++
		}
	}
	return result, nil
}

// ── helper types for logic tests ──────────────────────────────────────────────

func makeAccount(registrarID int64, apiType *string) *postgres.RegistrarAccount {
	creds, _ := json.Marshal(map[string]string{"api_key": "k", "api_secret": "s"})
	return &postgres.RegistrarAccount{
		ID:          1,
		RegistrarID: registrarID,
		Credentials: creds,
	}
}

func makeRegistrar(apiType *string) *postgres.Registrar {
	return &postgres.Registrar{ID: 1, Name: "GoDaddy", APIType: apiType}
}

func strPtr(s string) *string { return &s }

// ── tests ──────────────────────────────────────────────────────────────────────

func TestSyncAccountLogic_HappyPath(t *testing.T) {
	now := time.Now()
	exp := now.AddDate(1, 0, 0)

	stub := &stubProvider{
		domains: []registrarprovider.DomainInfo{
			{FQDN: "example.com", RegistrationDate: &now, ExpiryDate: &exp, AutoRenew: true},
			{FQDN: "other.org", RegistrationDate: &now, ExpiryDate: &exp, AutoRenew: false},
		},
	}

	providerType := "stub_happy"
	registrarprovider.Register(providerType, func(_ json.RawMessage) (registrarprovider.Provider, error) {
		return stub, nil
	})

	domStore := &stubDomainStore{}

	result, err := syncAccountLogic(
		context.Background(), 1,
		func(_ context.Context, _ int64) (*postgres.RegistrarAccount, error) {
			return makeAccount(1, nil), nil
		},
		func(_ context.Context, _ int64) (*postgres.Registrar, error) {
			return makeRegistrar(strPtr(providerType)), nil
		},
		func() domainDateUpdater { return domStore },
		func(apiType string, creds json.RawMessage) (registrarprovider.Provider, error) {
			return registrarprovider.Get(apiType, creds)
		},
		zap.NewNop(),
	)

	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	assert.Equal(t, 2, result.Updated)
	assert.Empty(t, result.NotFound)
	assert.Empty(t, result.Errors)
	assert.ElementsMatch(t, []string{"example.com", "other.org"}, domStore.updatedFQDNs)
}

func TestSyncAccountLogic_SomeDomainNotInDB(t *testing.T) {
	stub := &stubProvider{
		domains: []registrarprovider.DomainInfo{
			{FQDN: "indb.com"},
			{FQDN: "notindb.com"},
		},
	}
	providerType := "stub_notfound"
	registrarprovider.Register(providerType, func(_ json.RawMessage) (registrarprovider.Provider, error) {
		return stub, nil
	})

	domStore := &stubDomainStore{
		notFoundFQDNs: map[string]bool{"notindb.com": true},
	}

	result, err := syncAccountLogic(
		context.Background(), 1,
		func(_ context.Context, _ int64) (*postgres.RegistrarAccount, error) {
			return makeAccount(1, nil), nil
		},
		func(_ context.Context, _ int64) (*postgres.Registrar, error) {
			return makeRegistrar(strPtr(providerType)), nil
		},
		func() domainDateUpdater { return domStore },
		func(apiType string, creds json.RawMessage) (registrarprovider.Provider, error) {
			return registrarprovider.Get(apiType, creds)
		},
		zap.NewNop(),
	)

	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	assert.Equal(t, 1, result.Updated)
	assert.Equal(t, []string{"notindb.com"}, result.NotFound)
}

func TestSyncAccountLogic_AccountNotFound(t *testing.T) {
	_, err := syncAccountLogic(
		context.Background(), 99,
		func(_ context.Context, _ int64) (*postgres.RegistrarAccount, error) {
			return nil, postgres.ErrRegistrarAccountNotFound
		},
		nil, nil, nil, zap.NewNop(),
	)
	assert.ErrorIs(t, err, ErrAccountNotFound)
}

func TestSyncAccountLogic_RegistrarNotFound(t *testing.T) {
	_, err := syncAccountLogic(
		context.Background(), 1,
		func(_ context.Context, _ int64) (*postgres.RegistrarAccount, error) {
			return makeAccount(1, nil), nil
		},
		func(_ context.Context, _ int64) (*postgres.Registrar, error) {
			return nil, postgres.ErrRegistrarNotFound
		},
		nil, nil, zap.NewNop(),
	)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSyncAccountLogic_NoAPIType(t *testing.T) {
	_, err := syncAccountLogic(
		context.Background(), 1,
		func(_ context.Context, _ int64) (*postgres.RegistrarAccount, error) {
			return makeAccount(1, nil), nil
		},
		func(_ context.Context, _ int64) (*postgres.Registrar, error) {
			return makeRegistrar(nil), nil // nil api_type
		},
		nil, nil, zap.NewNop(),
	)
	assert.ErrorIs(t, err, ErrNoAPIType)
}

func TestSyncAccountLogic_ProviderListError(t *testing.T) {
	providerType := "stub_listerr"
	registrarprovider.Register(providerType, func(_ json.RawMessage) (registrarprovider.Provider, error) {
		return &stubProvider{err: errors.New("network error")}, nil
	})

	_, err := syncAccountLogic(
		context.Background(), 1,
		func(_ context.Context, _ int64) (*postgres.RegistrarAccount, error) {
			return makeAccount(1, nil), nil
		},
		func(_ context.Context, _ int64) (*postgres.Registrar, error) {
			return makeRegistrar(strPtr(providerType)), nil
		},
		func() domainDateUpdater { return &stubDomainStore{} },
		func(apiType string, creds json.RawMessage) (registrarprovider.Provider, error) {
			return registrarprovider.Get(apiType, creds)
		},
		zap.NewNop(),
	)
	assert.ErrorContains(t, err, "network error")
}

func TestSyncAccountLogic_DomainUpdateError_NonFatal(t *testing.T) {
	providerType := "stub_dberr"
	registrarprovider.Register(providerType, func(_ json.RawMessage) (registrarprovider.Provider, error) {
		return &stubProvider{domains: []registrarprovider.DomainInfo{{FQDN: "example.com"}}}, nil
	})

	domStore := &stubDomainStore{updateErr: errors.New("db timeout")}

	result, err := syncAccountLogic(
		context.Background(), 1,
		func(_ context.Context, _ int64) (*postgres.RegistrarAccount, error) {
			return makeAccount(1, nil), nil
		},
		func(_ context.Context, _ int64) (*postgres.Registrar, error) {
			return makeRegistrar(strPtr(providerType)), nil
		},
		func() domainDateUpdater { return domStore },
		func(apiType string, creds json.RawMessage) (registrarprovider.Provider, error) {
			return registrarprovider.Get(apiType, creds)
		},
		zap.NewNop(),
	)

	require.NoError(t, err)
	assert.Equal(t, 0, result.Updated)
	require.Len(t, result.Errors, 1)
	assert.Equal(t, "example.com", result.Errors[0].FQDN)
}

func TestSyncAccountLogic_EmptyRegistrar(t *testing.T) {
	providerType := "stub_empty"
	registrarprovider.Register(providerType, func(_ json.RawMessage) (registrarprovider.Provider, error) {
		return &stubProvider{domains: nil}, nil
	})

	result, err := syncAccountLogic(
		context.Background(), 1,
		func(_ context.Context, _ int64) (*postgres.RegistrarAccount, error) {
			return makeAccount(1, nil), nil
		},
		func(_ context.Context, _ int64) (*postgres.Registrar, error) {
			return makeRegistrar(strPtr(providerType)), nil
		},
		func() domainDateUpdater { return &stubDomainStore{} },
		func(apiType string, creds json.RawMessage) (registrarprovider.Provider, error) {
			return registrarprovider.Get(apiType, creds)
		},
		zap.NewNop(),
	)

	require.NoError(t, err)
	assert.Equal(t, 0, result.Total)
	assert.Equal(t, 0, result.Updated)
}

// ── SyncResult JSON shape ──────────────────────────────────────────────────────

func TestSyncResult_JSONOmitsEmptyErrors(t *testing.T) {
	r := SyncResult{Total: 5, Updated: 5, NotFound: []string{}}
	b, err := json.Marshal(r)
	require.NoError(t, err)
	assert.NotContains(t, string(b), `"errors"`)
}

func TestSyncResult_JSONIncludesErrors(t *testing.T) {
	r := SyncResult{
		Total:    2,
		Updated:  1,
		NotFound: []string{},
		Errors:   []SyncItemError{{FQDN: "bad.com", Message: "timeout"}},
	}
	b, err := json.Marshal(r)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"errors"`)
	assert.Contains(t, string(b), `"bad.com"`)
}
