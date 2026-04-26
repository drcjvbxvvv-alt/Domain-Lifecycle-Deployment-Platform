package lifecycle

// Unit tests for PE.3 domain list enrichment.
//
// These tests verify the new filter fields (CDNProviderID, Purpose) on
// ListInput, and the structure of ListEnrichedResult / DomainListRow.
//
// No database is required: we test only struct field presence and the
// toListFilter helper that converts ListInput → postgres.ListFilter.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"domain-platform/store/postgres"
)

// ── ListInput new fields ──────────────────────────────────────────────────────

func TestListInput_CDNProviderID_Field(t *testing.T) {
	id := int64(3)
	in := ListInput{CDNProviderID: &id}
	require.NotNil(t, in.CDNProviderID)
	assert.Equal(t, id, *in.CDNProviderID)
}

func TestListInput_CDNProviderID_NilByDefault(t *testing.T) {
	in := ListInput{}
	assert.Nil(t, in.CDNProviderID)
}

func TestListInput_Purpose_Field(t *testing.T) {
	p := "直播"
	in := ListInput{Purpose: &p}
	require.NotNil(t, in.Purpose)
	assert.Equal(t, "直播", *in.Purpose)
}

func TestListInput_Purpose_NilByDefault(t *testing.T) {
	in := ListInput{}
	assert.Nil(t, in.Purpose)
}

// ── toListFilter mapping ──────────────────────────────────────────────────────

func TestToListFilter_CDNProviderID(t *testing.T) {
	id := int64(7)
	f := toListFilter(ListInput{CDNProviderID: &id})
	require.NotNil(t, f.CDNProviderID)
	assert.Equal(t, id, *f.CDNProviderID)
}

func TestToListFilter_Purpose(t *testing.T) {
	p := "備用"
	f := toListFilter(ListInput{Purpose: &p})
	require.NotNil(t, f.Purpose)
	assert.Equal(t, "備用", *f.Purpose)
}

func TestToListFilter_AllNilWhenEmpty(t *testing.T) {
	f := toListFilter(ListInput{})
	assert.Nil(t, f.CDNProviderID)
	assert.Nil(t, f.Purpose)
	assert.Nil(t, f.CDNAccountID)
	assert.Nil(t, f.RegistrarID)
}

func TestToListFilter_PreservesExistingFields(t *testing.T) {
	pid := int64(1)
	reg := int64(2)
	cdn := int64(3)
	f := toListFilter(ListInput{
		ProjectID:   &pid,
		RegistrarID: &reg,
		CDNAccountID: &cdn,
	})
	require.NotNil(t, f.ProjectID)
	require.NotNil(t, f.RegistrarID)
	require.NotNil(t, f.CDNAccountID)
	assert.Equal(t, pid, *f.ProjectID)
	assert.Equal(t, reg, *f.RegistrarID)
	assert.Equal(t, cdn, *f.CDNAccountID)
}

// ── DomainListRow struct field presence ──────────────────────────────────────

func TestDomainListRow_RegistrarName_Nil(t *testing.T) {
	row := postgres.DomainListRow{}
	assert.Nil(t, row.RegistrarName)
}

func TestDomainListRow_RegistrarName_Set(t *testing.T) {
	name := "GoDaddy"
	row := postgres.DomainListRow{RegistrarName: &name}
	require.NotNil(t, row.RegistrarName)
	assert.Equal(t, "GoDaddy", *row.RegistrarName)
}

func TestDomainListRow_CDNAccountName_Nil(t *testing.T) {
	row := postgres.DomainListRow{}
	assert.Nil(t, row.CDNAccountName)
}

func TestDomainListRow_CDNAccountName_Set(t *testing.T) {
	name := "主播 Cloudflare"
	row := postgres.DomainListRow{CDNAccountName: &name}
	require.NotNil(t, row.CDNAccountName)
	assert.Equal(t, "主播 Cloudflare", *row.CDNAccountName)
}

func TestDomainListRow_CDNProviderType_Nil(t *testing.T) {
	row := postgres.DomainListRow{}
	assert.Nil(t, row.CDNProviderType)
}

func TestDomainListRow_CDNProviderType_Set(t *testing.T) {
	pt := "cloudflare"
	row := postgres.DomainListRow{CDNProviderType: &pt}
	require.NotNil(t, row.CDNProviderType)
	assert.Equal(t, "cloudflare", *row.CDNProviderType)
}

func TestDomainListRow_EmbedsDomain(t *testing.T) {
	// Verify the Domain fields are accessible through the embedded struct.
	row := postgres.DomainListRow{}
	row.FQDN = "example.com"
	row.ProjectID = 42
	assert.Equal(t, "example.com", row.FQDN)
	assert.Equal(t, int64(42), row.ProjectID)
}

// ── Table-driven: all enriched fields round-trip through DomainListRow ───────

func TestDomainListRow_AllEnrichedFields_TableDriven(t *testing.T) {
	regName    := "Namecheap"
	cdnAccount := "主播 CF"
	cdnType    := "cloudflare"

	cases := []struct {
		name            string
		registrarName   *string
		cdnAccountName  *string
		cdnProviderType *string
	}{
		{"all nil",          nil,       nil,        nil},
		{"registrar only",   &regName,  nil,        nil},
		{"cdn account only", nil,       &cdnAccount, nil},
		{"cdn type only",    nil,       nil,        &cdnType},
		{"all set",          &regName,  &cdnAccount, &cdnType},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := postgres.DomainListRow{
				RegistrarName:   tc.registrarName,
				CDNAccountName:  tc.cdnAccountName,
				CDNProviderType: tc.cdnProviderType,
			}
			assert.Equal(t, tc.registrarName,   row.RegistrarName)
			assert.Equal(t, tc.cdnAccountName,  row.CDNAccountName)
			assert.Equal(t, tc.cdnProviderType, row.CDNProviderType)
		})
	}
}

// ── ListEnrichedResult struct ─────────────────────────────────────────────────

func TestListEnrichedResult_Fields(t *testing.T) {
	r := ListEnrichedResult{
		Items:  []postgres.DomainListRow{{}, {}},
		Total:  100,
		Cursor: 42,
	}
	assert.Len(t, r.Items, 2)
	assert.Equal(t, int64(100), r.Total)
	assert.Equal(t, int64(42), r.Cursor)
}
