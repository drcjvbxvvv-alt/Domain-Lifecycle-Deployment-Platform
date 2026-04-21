package cost

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── normalizeTLD ──────────────────────────────────────────────────────────────

func TestNormalizeTLD(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{".com", ".com"},
		{"com", ".com"},
		{".COM", ".com"},
		{"COM", ".com"},
		{".co.uk", ".co.uk"},
		{"co.uk", ".co.uk"},
		{"  .io  ", ".io"},
		{"", ""},
		{" ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeTLD(tt.input))
		})
	}
}

// ── validateCurrency ──────────────────────────────────────────────────────────

func TestValidateCurrency(t *testing.T) {
	valid := []string{"USD", "EUR", "GBP", "CNY", "TWD", "JPY", "AUD", "CAD"}
	for _, code := range valid {
		t.Run("valid_"+code, func(t *testing.T) {
			assert.NoError(t, validateCurrency(code))
		})
	}

	invalid := []string{"", "US", "USDT", "xyz"} // note: lowercase accepted (uppercased internally)
	for _, code := range invalid {
		t.Run("invalid_"+code, func(t *testing.T) {
			err := validateCurrency(code)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidCurrency)
		})
	}
}

// ── ValidCostTypes ────────────────────────────────────────────────────────────

func TestValidCostTypes(t *testing.T) {
	valid := []string{"registration", "renewal", "transfer", "privacy", "other"}
	for _, ct := range valid {
		assert.True(t, ValidCostTypes[ct], "expected %q to be a valid cost type", ct)
	}

	invalid := []string{"", "annual", "purchase", "RENEWAL"}
	for _, ct := range invalid {
		assert.False(t, ValidCostTypes[ct], "expected %q to be invalid", ct)
	}
}

// ── sentinel distinctness ─────────────────────────────────────────────────────

func TestSentinelErrors(t *testing.T) {
	errs := []error{ErrFeeScheduleNotFound, ErrCostNotFound, ErrInvalidCostType, ErrInvalidCurrency}
	seen := make(map[string]bool)
	for _, e := range errs {
		msg := e.Error()
		assert.False(t, seen[msg], "duplicate sentinel error message: %q", msg)
		assert.NotEmpty(t, msg)
		seen[msg] = true
	}
}

// ── normalizeTLD + fee schedule TLD interaction ───────────────────────────────

func TestNormalizeTLDIdempotent(t *testing.T) {
	// Applying normalizeTLD twice should yield the same result as once.
	inputs := []string{".com", "com", ".co.uk", "co.uk", "IO", ".io"}
	for _, in := range inputs {
		once := normalizeTLD(in)
		twice := normalizeTLD(once)
		assert.Equal(t, once, twice, "normalizeTLD should be idempotent for %q", in)
	}
}
