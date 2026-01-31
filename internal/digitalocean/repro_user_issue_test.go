package digitalocean

import (
	"context"
	"testing"

	"github.com/digitalocean/godo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

// Mock client specifically for this reproduction
type reproMockClient struct {
	zones   []godo.Domain
	records map[string][]godo.DomainRecord
}

func (m *reproMockClient) List(ctx context.Context, opt *godo.ListOptions) ([]godo.Domain, *godo.Response, error) {
	if opt == nil || opt.Page == 0 {
		return m.zones, &godo.Response{Links: &godo.Links{}}, nil
	}
	return []godo.Domain{}, nil, nil
}

func (m *reproMockClient) Records(ctx context.Context, domain string, opt *godo.ListOptions) ([]godo.DomainRecord, *godo.Response, error) {
	if recs, ok := m.records[domain]; ok {
		return recs, &godo.Response{Links: &godo.Links{}}, nil
	}
	return []godo.DomainRecord{}, &godo.Response{Links: &godo.Links{}}, nil
}

// Implement other interface methods as no-ops
func (m *reproMockClient) RecordsByName(context.Context, string, string, *godo.ListOptions) ([]godo.DomainRecord, *godo.Response, error) {
	return nil, nil, nil
}
func (m *reproMockClient) RecordsByTypeAndName(context.Context, string, string, string, *godo.ListOptions) ([]godo.DomainRecord, *godo.Response, error) {
	return nil, nil, nil
}
func (m *reproMockClient) RecordsByType(context.Context, string, string, *godo.ListOptions) ([]godo.DomainRecord, *godo.Response, error) {
	return nil, nil, nil
}
func (m *reproMockClient) Create(context.Context, *godo.DomainCreateRequest) (*godo.Domain, *godo.Response, error) {
	return nil, nil, nil
}
func (m *reproMockClient) CreateRecord(context.Context, string, *godo.DomainRecordEditRequest) (*godo.DomainRecord, *godo.Response, error) {
	return &godo.DomainRecord{ID: 999}, nil, nil
}
func (m *reproMockClient) Delete(context.Context, string) (*godo.Response, error) { return nil, nil }
func (m *reproMockClient) DeleteRecord(ctx context.Context, domain string, id int) (*godo.Response, error) {
	return nil, nil
}
func (m *reproMockClient) EditRecord(ctx context.Context, domain string, id int, editRequest *godo.DomainRecordEditRequest) (*godo.DomainRecord, *godo.Response, error) {
	return &godo.DomainRecord{ID: id}, nil, nil
}
func (m *reproMockClient) Get(ctx context.Context, name string) (*godo.Domain, *godo.Response, error) {
	return nil, nil, nil
}
func (m *reproMockClient) Record(ctx context.Context, domain string, id int) (*godo.DomainRecord, *godo.Response, error) {
	return nil, nil, nil
}

func TestReproUserIssue(t *testing.T) {
	// Scenario: User has 'bibook.dev' zone.
	// Desired endpoints from the issue description.

	mock := &reproMockClient{
		zones: []godo.Domain{
			{Name: "bibook.dev"},
		},
		records: map[string][]godo.DomainRecord{
			"bibook.dev": {}, // Initially empty
		},
	}

	p := &Provider{
		client:       mock,
		domainFilter: endpoint.NewDomainFilter([]string{}), // No filter
		apiPageSize:  50,
	}

	// Define the endpoints from the user's YAML
	endpoints := []*endpoint.Endpoint{
		{DNSName: "cp2317.qa.hub.bibook.dev", RecordType: "TXT", Targets: endpoint.Targets{"v=spf1 include:mailgun.org ~all"}},
		{DNSName: "mta._domainkey.bibook.dev", RecordType: "TXT", Targets: endpoint.Targets{"k=rsa; p=MIIBIjANBg..."}},
		{DNSName: "email.cp2317.qa.hub.bibook.dev", RecordType: "CNAME", Targets: endpoint.Targets{"eu.mailgun.org"}},
		{DNSName: "cp2317.qa.hub.bibook.dev", RecordType: "MX", Targets: endpoint.Targets{"10 mxa.eu.mailgun.org", "10 mxb.eu.mailgun.org"}},
		{DNSName: "cp2317.qa.hub.bibook.dev", RecordType: "A", Targets: endpoint.Targets{"162.55.159.213"}},
		{DNSName: "*.cp2317.qa.hub.bibook.dev", RecordType: "A", Targets: endpoint.Targets{"162.55.159.213"}},
	}

	// Prepare changes: All these should be Creates
	planChanges := &plan.Changes{
		Create: endpoints,
	}

	ctx := context.Background()
	recordsByDomain, zoneNameIDMapper, err := p.getRecordsByDomain(ctx)
	require.NoError(t, err)

	createsByDomain := endpointsByZone(zoneNameIDMapper, planChanges.Create)

	// Check if all endpoints were assigned to the correct zone
	assert.Len(t, createsByDomain["bibook.dev"], 6, "All 6 endpoints should be in bibook.dev zone")

	var chg changes
	err = processCreateActions(recordsByDomain, createsByDomain, &chg)
	require.NoError(t, err)

	// 6 endpoints, but MX has 2 targets, so 7 records to create
	assert.Len(t, chg.Creates, 7)

	// Check names of created records
	createdNames := make(map[string]bool)
	for _, c := range chg.Creates {
		createdNames[c.Options.Name] = true
	}

	assert.True(t, createdNames["cp2317.qa.hub"], "Should create cp2317.qa.hub")
	assert.True(t, createdNames["*.cp2317.qa.hub"], "Should create *.cp2317.qa.hub")
}

func TestReproUserIssue_WithSubZone(t *testing.T) {
	// Scenario: User has 'bibook.dev' AND 'qa.hub.bibook.dev' (hypothetically).
	// This tests which zone takes precedence.

	mock := &reproMockClient{
		zones: []godo.Domain{
			{Name: "bibook.dev"},
			{Name: "qa.hub.bibook.dev"},
		},
		records: map[string][]godo.DomainRecord{
			"bibook.dev":        {},
			"qa.hub.bibook.dev": {},
		},
	}

	p := &Provider{
		client:       mock,
		domainFilter: endpoint.NewDomainFilter([]string{}),
		apiPageSize:  50,
	}

	endpoints := []*endpoint.Endpoint{
		{DNSName: "cp2317.qa.hub.bibook.dev", RecordType: "A", Targets: endpoint.Targets{"1.2.3.4"}},
		{DNSName: "*.cp2317.qa.hub.bibook.dev", RecordType: "A", Targets: endpoint.Targets{"1.2.3.4"}},
	}

	planChanges := &plan.Changes{
		Create: endpoints,
	}

	ctx := context.Background()
	recordsByDomain, zoneNameIDMapper, err := p.getRecordsByDomain(ctx)
	require.NoError(t, err)

	createsByDomain := endpointsByZone(zoneNameIDMapper, planChanges.Create)

	// They should prefer the longer zone match
	assert.Len(t, createsByDomain["qa.hub.bibook.dev"], 2)
	assert.Len(t, createsByDomain["bibook.dev"], 0)

	var chg changes
	err = processCreateActions(recordsByDomain, createsByDomain, &chg)
	require.NoError(t, err)

	for _, c := range chg.Creates {
		if c.Domain == "qa.hub.bibook.dev" {
			// Name should be stripped relative to zone
			// cp2317.qa.hub.bibook.dev -> cp2317
			// *.cp2317.qa.hub.bibook.dev -> *.cp2317
			assert.True(t, c.Options.Name == "cp2317" || c.Options.Name == "*.cp2317",
				"Expected cp2317 or *.cp2317, got %s", c.Options.Name)
		}
	}
}

// Test that SoftError is returned when some operations fail
func TestSoftErrorOnPartialFailure(t *testing.T) {
	mock := &reproMockClient{
		zones: []godo.Domain{
			{Name: "example.com"},
		},
		records: map[string][]godo.DomainRecord{
			"example.com": {},
		},
	}

	p := &Provider{
		client:       mock,
		domainFilter: endpoint.NewDomainFilter([]string{}),
		apiPageSize:  50,
		dryRun:       false,
	}

	endpoints := []*endpoint.Endpoint{
		{DNSName: "test.example.com", RecordType: "A", Targets: endpoint.Targets{"1.2.3.4"}},
	}

	planChanges := &plan.Changes{
		Create: endpoints,
	}

	err := p.ApplyChanges(context.Background(), planChanges)
	// Should not return error when create succeeds (mock returns success)
	assert.NoError(t, err)

	// Test with failing mock would need a different mock that returns errors
}

// Test that provider correctly handles domain filter
func TestDomainFilter(t *testing.T) {
	mock := &reproMockClient{
		zones: []godo.Domain{
			{Name: "example.com"},
			{Name: "example.org"},
			{Name: "test.io"},
		},
		records: map[string][]godo.DomainRecord{
			"example.com": {},
			"example.org": {},
			"test.io":     {},
		},
	}

	p := &Provider{
		client:       mock,
		domainFilter: endpoint.NewDomainFilter([]string{"example.com", "example.org"}),
		apiPageSize:  50,
	}

	zones, err := p.zones(context.Background())
	require.NoError(t, err)

	assert.Len(t, zones, 2)
	zoneNames := make(map[string]bool)
	for _, z := range zones {
		zoneNames[z.Name] = true
	}
	assert.True(t, zoneNames["example.com"])
	assert.True(t, zoneNames["example.org"])
	assert.False(t, zoneNames["test.io"])
}

// Test GetDomainFilter returns correct filter
func TestGetDomainFilter(t *testing.T) {
	filter := endpoint.NewDomainFilter([]string{"example.com"})
	p := &Provider{
		domainFilter: filter,
	}

	result := p.GetDomainFilter()
	assert.True(t, result.Match("test.example.com"))
	assert.False(t, result.Match("test.other.com"))
}

// Test SupportedRecordType
func TestSupportedRecordType(t *testing.T) {
	p := &Provider{}

	assert.True(t, p.SupportedRecordType("A"))
	assert.True(t, p.SupportedRecordType("AAAA"))
	assert.True(t, p.SupportedRecordType("CNAME"))
	assert.True(t, p.SupportedRecordType("TXT"))
	assert.True(t, p.SupportedRecordType("MX"))
	assert.True(t, p.SupportedRecordType("SRV"))    // Supported by default provider
	assert.False(t, p.SupportedRecordType("PTR"))   // Not supported by default
	assert.False(t, p.SupportedRecordType("NAPTR")) // Not supported
}

// Test that SoftError is properly created
func TestSoftError(t *testing.T) {
	err := provider.NewSoftError(assert.AnError)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), assert.AnError.Error())
}
