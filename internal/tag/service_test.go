package tag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateColor(t *testing.T) {
	valid := []string{"#000000", "#FFFFFF", "#ff00aa", "#1a2B3c"}
	for _, c := range valid {
		t.Run("valid_"+c, func(t *testing.T) {
			s := c
			assert.NoError(t, ValidateColor(&s))
		})
	}

	invalid := []string{"#FFF", "#GGGGGG", "000000", "#12345", "#1234567", "red", ""}
	for _, c := range invalid {
		t.Run("invalid_"+c, func(t *testing.T) {
			s := c
			if s == "" {
				// Empty string → nil equivalent → should be OK
				assert.NoError(t, ValidateColor(&s))
				return
			}
			err := ValidateColor(&s)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidColor)
		})
	}

	// nil colour is accepted
	t.Run("nil_is_ok", func(t *testing.T) {
		assert.NoError(t, ValidateColor(nil))
	})
}

func TestSentinelErrors(t *testing.T) {
	errs := []error{ErrNotFound, ErrDuplicateName, ErrEmptyName, ErrInvalidColor}
	seen := make(map[string]bool)
	for _, e := range errs {
		msg := e.Error()
		assert.False(t, seen[msg], "duplicate sentinel: %q", msg)
		assert.NotEmpty(t, msg)
		seen[msg] = true
	}
}

func TestColorRePattern(t *testing.T) {
	// Regression: ensure the regex matches exactly 6 hex digits after #
	assert.True(t, colorRe.MatchString("#abcdef"))
	assert.True(t, colorRe.MatchString("#ABCDEF"))
	assert.False(t, colorRe.MatchString("#abcde"))   // 5 digits
	assert.False(t, colorRe.MatchString("#abcdefg"))  // 7 digits
	assert.False(t, colorRe.MatchString("abcdef"))    // no #
}
