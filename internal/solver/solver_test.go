package solver

import (
	"context"
	"errors"
	"testing"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/rhpds/cert-manager-webhook-ibmcis/internal/cis"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

type mockCISClient struct {
	zones            []cis.Zone
	records          []cis.DNSRecord
	listZonesErr     error
	listRecordsErr   error
	createRecordErr  error
	deleteRecordErr  error
	createdRecords   []mockCreateCall
	deletedRecordIDs []string
}

type mockCreateCall struct {
	CRN, ZoneID, Name, Content string
}

func (m *mockCISClient) ListZones(_ context.Context, _ string) ([]cis.Zone, error) {
	return m.zones, m.listZonesErr
}

func (m *mockCISClient) ListTXTRecords(_ context.Context, _, _, _ string) ([]cis.DNSRecord, error) {
	return m.records, m.listRecordsErr
}

func (m *mockCISClient) CreateTXTRecord(_ context.Context, crn, zoneID, name, content string) error {
	m.createdRecords = append(m.createdRecords, mockCreateCall{crn, zoneID, name, content})
	return m.createRecordErr
}

func (m *mockCISClient) DeleteTXTRecord(_ context.Context, _, _, recordID string) error {
	m.deletedRecordIDs = append(m.deletedRecordIDs, recordID)
	return m.deleteRecordErr
}

func configJSON(t *testing.T, crns ...string) *extapi.JSON {
	t.Helper()
	if len(crns) == 0 {
		crns = []string{"crn:v1:bluemix:public:internet-svcs:global:a/abc123:def456::"}
	}
	raw := `{"cisCRN":[`
	for i, crn := range crns {
		if i > 0 {
			raw += ","
		}
		raw += `"` + crn + `"`
	}
	raw += `]}`
	return &extapi.JSON{Raw: []byte(raw)}
}

func challenge(fqdn, key string, cfg *extapi.JSON) *v1alpha1.ChallengeRequest {
	return &v1alpha1.ChallengeRequest{
		ResourceNamespace: "default",
		ResolvedZone:      "example.com.",
		ResolvedFQDN:      fqdn,
		Key:               key,
		Config:            cfg,
	}
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		input   *extapi.JSON
		wantErr bool
	}{
		{name: "nil config returns error", input: nil, wantErr: true},
		{name: "empty CRNs returns error", input: &extapi.JSON{Raw: []byte(`{"cisCRN":[]}`)}, wantErr: true},
		{name: "invalid CRN format returns error", input: &extapi.JSON{Raw: []byte(`{"cisCRN":["not-a-crn"]}`)}, wantErr: true},
		{name: "valid config succeeds", input: configJSON(t), wantErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadConfig(tc.input)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPresent(t *testing.T) {
	tests := []struct {
		name    string
		mock    *mockCISClient
		ch      *v1alpha1.ChallengeRequest
		wantErr bool
	}{
		{
			name: "successful present",
			mock: &mockCISClient{zones: []cis.Zone{{ID: "zone-1", Name: "example.com"}}},
			ch:   challenge("_acme-challenge.example.com.", "test-key", configJSON(t)),
		},
		{
			name:    "create record fails returns error",
			mock:    &mockCISClient{zones: []cis.Zone{{ID: "zone-1", Name: "example.com"}}, createRecordErr: errors.New("api error")},
			ch:      challenge("_acme-challenge.example.com.", "test-key", configJSON(t)),
			wantErr: true,
		},
		{
			name:    "no matching zone returns error",
			mock:    &mockCISClient{zones: []cis.Zone{{ID: "zone-1", Name: "other.com"}}},
			ch:      challenge("_acme-challenge.example.com.", "test-key", configJSON(t)),
			wantErr: true,
		},
		{
			name:    "nil config returns error",
			mock:    &mockCISClient{},
			ch:      challenge("_acme-challenge.example.com.", "test-key", nil),
			wantErr: true,
		},
		{
			name:    "empty CRNs returns error",
			mock:    &mockCISClient{},
			ch:      challenge("_acme-challenge.example.com.", "test-key", &extapi.JSON{Raw: []byte(`{"cisCRN":[]}`)}),
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := New(tc.mock)
			err := s.Present(tc.ch)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCleanUp(t *testing.T) {
	tests := []struct {
		name           string
		mock           *mockCISClient
		ch             *v1alpha1.ChallengeRequest
		wantErr        bool
		wantDeletedIDs []string
	}{
		{
			name: "successful cleanup deletes matching record",
			mock: &mockCISClient{
				zones: []cis.Zone{{ID: "zone-1", Name: "example.com"}},
				records: []cis.DNSRecord{
					{ID: "rec-1", Name: "_acme-challenge.example.com", Content: "test-key"},
					{ID: "rec-2", Name: "_acme-challenge.example.com", Content: "other-key"},
				},
			},
			ch:             challenge("_acme-challenge.example.com.", "test-key", configJSON(t)),
			wantDeletedIDs: []string{"rec-1"},
		},
		{
			name: "no matching record is idempotent",
			mock: &mockCISClient{zones: []cis.Zone{{ID: "zone-1", Name: "example.com"}}, records: []cis.DNSRecord{}},
			ch:   challenge("_acme-challenge.example.com.", "test-key", configJSON(t)),
		},
		{
			name: "delete fails returns error",
			mock: &mockCISClient{
				zones:           []cis.Zone{{ID: "zone-1", Name: "example.com"}},
				records:         []cis.DNSRecord{{ID: "rec-1", Name: "_acme-challenge.example.com", Content: "test-key"}},
				deleteRecordErr: errors.New("api error"),
			},
			ch:             challenge("_acme-challenge.example.com.", "test-key", configJSON(t)),
			wantErr:        true,
			wantDeletedIDs: []string{"rec-1"},
		},
		{
			name:    "no matching zone returns error",
			mock:    &mockCISClient{zones: []cis.Zone{{ID: "zone-1", Name: "other.com"}}},
			ch:      challenge("_acme-challenge.example.com.", "test-key", configJSON(t)),
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := New(tc.mock)
			err := s.CleanUp(tc.ch)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(tc.wantDeletedIDs) != len(tc.mock.deletedRecordIDs) {
				t.Fatalf("expected %d deletions, got %d", len(tc.wantDeletedIDs), len(tc.mock.deletedRecordIDs))
			}
			for i, id := range tc.wantDeletedIDs {
				if tc.mock.deletedRecordIDs[i] != id {
					t.Errorf("deleted[%d] = %s, want %s", i, tc.mock.deletedRecordIDs[i], id)
				}
			}
		})
	}
}

func TestNormalizeFQDN(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"example.com", "example.com."},
		{"example.com.", "example.com."},
		{"_acme-challenge.sub.example.com", "_acme-challenge.sub.example.com."},
	}
	for _, tc := range tests {
		got := normalizeFQDN(tc.input)
		if got != tc.want {
			t.Errorf("normalizeFQDN(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
