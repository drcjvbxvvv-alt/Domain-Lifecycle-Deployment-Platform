// Package dns defines the DNS provider abstraction layer.
//
// A Provider represents a DNS hosting service (Cloudflare, Route53, etc.)
// that can list, create, update, and delete DNS records via its API.
// The platform uses this to:
//  1. Fetch "expected" records (what the provider says should exist)
//  2. Compare against live DNS resolution (drift detection)
//  3. Manage records programmatically (future: auto-provision)
//
// Each provider implementation reads its config and credentials from the
// dns_providers table's config/credentials JSONB columns.
package dns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// ── Errors ────────────────────────────────────────────────────────────────────

var (
	ErrProviderNotRegistered = errors.New("dns provider type not registered")
	ErrMissingCredentials    = errors.New("dns provider credentials missing or invalid")
	ErrMissingConfig         = errors.New("dns provider config missing or invalid")
	ErrZoneNotFound          = errors.New("dns zone not found")
	ErrRecordNotFound        = errors.New("dns record not found")
)

// ── Record ────────────────────────────────────────────────────────────────────

// Record represents a single DNS record as stored by a DNS provider.
type Record struct {
	ID       string `json:"id"`                 // provider-specific record ID
	Type     string `json:"type"`               // A, AAAA, CNAME, MX, TXT, etc.
	Name     string `json:"name"`               // full record name (e.g. "shop.example.com")
	Content  string `json:"content"`            // record value
	TTL      int    `json:"ttl"`                // seconds; 1 = automatic (Cloudflare)
	Priority int    `json:"priority,omitempty"` // MX, SRV
	Proxied  bool   `json:"proxied,omitempty"`  // Cloudflare-specific
}

// RecordFilter limits which records are returned by ListRecords.
type RecordFilter struct {
	Type string // filter by record type (empty = all)
	Name string // filter by record name (empty = all in zone)
}

// ── Provider interface ────────────────────────────────────────────────────────

// Provider is the abstraction for a DNS hosting provider's API.
type Provider interface {
	// Name returns the provider type identifier (e.g. "cloudflare").
	Name() string

	// ListRecords returns all DNS records matching the filter.
	// zone is provider-specific (e.g. Cloudflare zone ID, Route53 hosted zone ID).
	ListRecords(ctx context.Context, zone string, filter RecordFilter) ([]Record, error)

	// CreateRecord creates a new DNS record in the zone.
	CreateRecord(ctx context.Context, zone string, record Record) (*Record, error)

	// UpdateRecord updates an existing DNS record by its provider-specific ID.
	UpdateRecord(ctx context.Context, zone string, recordID string, record Record) (*Record, error)

	// DeleteRecord removes a DNS record by its provider-specific ID.
	DeleteRecord(ctx context.Context, zone string, recordID string) error
}

// ── Factory ───────────────────────────────────────────────────────────────────

// Factory creates a Provider instance from config and credentials JSON.
type Factory func(config, credentials json.RawMessage) (Provider, error)

// ── Registry ──────────────────────────────────────────────────────────────────

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Factory)
)

// Register adds a provider factory to the global registry.
// Called in init() of each provider implementation file.
func Register(providerType string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[providerType] = factory
}

// Get creates a Provider instance for the given type, using the provided
// config and credentials JSON. Returns ErrProviderNotRegistered if the
// type has no registered factory.
func Get(providerType string, config, credentials json.RawMessage) (Provider, error) {
	registryMu.RLock()
	factory, ok := registry[providerType]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotRegistered, providerType)
	}
	return factory(config, credentials)
}

// RegisteredTypes returns the list of provider types that have been registered.
func RegisteredTypes() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	types := make([]string, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	return types
}
