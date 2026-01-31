/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package digitalocean

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/digitalocean/godo"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

type mockDigitalOceanClient struct{}

func (m *mockDigitalOceanClient) RecordsByName(context.Context, string, string, *godo.ListOptions) ([]godo.DomainRecord, *godo.Response, error) {
	return nil, nil, nil
}

func (m *mockDigitalOceanClient) RecordsByTypeAndName(context.Context, string, string, string, *godo.ListOptions) ([]godo.DomainRecord, *godo.Response, error) {
	return nil, nil, nil
}

func (m *mockDigitalOceanClient) RecordsByType(context.Context, string, string, *godo.ListOptions) ([]godo.DomainRecord, *godo.Response, error) {
	return nil, nil, nil
}

func (m *mockDigitalOceanClient) List(ctx context.Context, opt *godo.ListOptions) ([]godo.Domain, *godo.Response, error) {
	if opt == nil || opt.Page == 0 {
		return []godo.Domain{{Name: "foo.com"}, {Name: "example.com"}}, &godo.Response{
			Links: &godo.Links{
				Pages: &godo.Pages{
					Next: "http://example.com/v2/domains/?page=2",
					Last: "http://example.com/v2/domains/?page=2",
				},
			},
		}, nil
	}
	return []godo.Domain{{Name: "bar.com"}, {Name: "bar.de"}}, nil, nil
}

func (m *mockDigitalOceanClient) Create(context.Context, *godo.DomainCreateRequest) (*godo.Domain, *godo.Response, error) {
	return &godo.Domain{Name: "example.com"}, nil, nil
}

func (m *mockDigitalOceanClient) CreateRecord(context.Context, string, *godo.DomainRecordEditRequest) (*godo.DomainRecord, *godo.Response, error) {
	return &godo.DomainRecord{ID: 1, Name: "new", Type: "CNAME"}, nil, nil
}

func (m *mockDigitalOceanClient) Delete(context.Context, string) (*godo.Response, error) {
	return nil, fmt.Errorf("failed to delete domain")
}

func (m *mockDigitalOceanClient) DeleteRecord(ctx context.Context, domain string, id int) (*godo.Response, error) {
	return nil, fmt.Errorf("failed to delete record")
}

func (m *mockDigitalOceanClient) EditRecord(ctx context.Context, domain string, id int, editRequest *godo.DomainRecordEditRequest) (*godo.DomainRecord, *godo.Response, error) {
	return &godo.DomainRecord{ID: 1}, nil, nil
}

func (m *mockDigitalOceanClient) Get(ctx context.Context, name string) (*godo.Domain, *godo.Response, error) {
	return &godo.Domain{Name: "example.com"}, nil, nil
}

func (m *mockDigitalOceanClient) Record(ctx context.Context, domain string, id int) (*godo.DomainRecord, *godo.Response, error) {
	return &godo.DomainRecord{ID: 1}, nil, nil
}

func (m *mockDigitalOceanClient) Records(ctx context.Context, domain string, opt *godo.ListOptions) ([]godo.DomainRecord, *godo.Response, error) {
	switch domain {
	case "foo.com":
		if opt == nil || opt.Page == 0 {
			return []godo.DomainRecord{
				{ID: 1, Name: "foo.ext-dns-test", Type: "CNAME"},
				{ID: 2, Name: "bar.ext-dns-test", Type: "CNAME"},
				{ID: 3, Name: "@", Type: endpoint.RecordTypeCNAME},
				{ID: 4, Name: "@", Type: endpoint.RecordTypeMX, Priority: 10, Data: "mx1.foo.com."},
				{ID: 5, Name: "@", Type: endpoint.RecordTypeMX, Priority: 10, Data: "mx2.foo.com."},
				{ID: 6, Name: "@", Type: endpoint.RecordTypeTXT, Data: "SOME-TXT-TEXT"},
			}, &godo.Response{
				Links: &godo.Links{
					Pages: &godo.Pages{
						Next: "http://example.com/v2/domains/?page=2",
						Last: "1234",
					},
				},
			}, nil
		}
		return []godo.DomainRecord{{ID: 3, Name: "baz.ext-dns-test", Type: "A"}}, nil, nil
	case "example.com":
		if opt == nil || opt.Page == 0 {
			return []godo.DomainRecord{{ID: 1, Name: "new", Type: "CNAME"}}, &godo.Response{
				Links: &godo.Links{
					Pages: &godo.Pages{
						Next: "http://example.com/v2/domains/?page=2",
						Last: "1234",
					},
				},
			}, nil
		}
		return nil, nil, nil
	default:
		return nil, nil, nil
	}
}

type mockDigitalOceanRecordsFail struct{}

func (m *mockDigitalOceanRecordsFail) RecordsByName(context.Context, string, string, *godo.ListOptions) ([]godo.DomainRecord, *godo.Response, error) {
	return nil, nil, nil
}

func (m *mockDigitalOceanRecordsFail) RecordsByTypeAndName(context.Context, string, string, string, *godo.ListOptions) ([]godo.DomainRecord, *godo.Response, error) {
	return nil, nil, nil
}

func (m *mockDigitalOceanRecordsFail) RecordsByType(context.Context, string, string, *godo.ListOptions) ([]godo.DomainRecord, *godo.Response, error) {
	return nil, nil, nil
}

func (m *mockDigitalOceanRecordsFail) List(context.Context, *godo.ListOptions) ([]godo.Domain, *godo.Response, error) {
	return []godo.Domain{{Name: "foo.com"}, {Name: "bar.com"}}, nil, nil
}

func (m *mockDigitalOceanRecordsFail) Create(context.Context, *godo.DomainCreateRequest) (*godo.Domain, *godo.Response, error) {
	return &godo.Domain{Name: "example.com"}, nil, nil
}

func (m *mockDigitalOceanRecordsFail) CreateRecord(context.Context, string, *godo.DomainRecordEditRequest) (*godo.DomainRecord, *godo.Response, error) {
	return &godo.DomainRecord{ID: 1}, nil, nil
}

func (m *mockDigitalOceanRecordsFail) Delete(context.Context, string) (*godo.Response, error) {
	return nil, fmt.Errorf("failed to delete record")
}

func (m *mockDigitalOceanRecordsFail) DeleteRecord(ctx context.Context, domain string, id int) (*godo.Response, error) {
	return nil, fmt.Errorf("failed to delete record")
}

func (m *mockDigitalOceanRecordsFail) EditRecord(ctx context.Context, domain string, id int, editRequest *godo.DomainRecordEditRequest) (*godo.DomainRecord, *godo.Response, error) {
	return &godo.DomainRecord{ID: 1}, nil, nil
}

func (m *mockDigitalOceanRecordsFail) Get(ctx context.Context, name string) (*godo.Domain, *godo.Response, error) {
	return &godo.Domain{Name: "example.com"}, nil, nil
}

func (m *mockDigitalOceanRecordsFail) Record(ctx context.Context, domain string, id int) (*godo.DomainRecord, *godo.Response, error) {
	return nil, nil, fmt.Errorf("Failed to get records")
}

func (m *mockDigitalOceanRecordsFail) Records(ctx context.Context, domain string, opt *godo.ListOptions) ([]godo.DomainRecord, *godo.Response, error) {
	return []godo.DomainRecord{}, nil, fmt.Errorf("Failed to get records")
}

func isEmpty(xs any) bool {
	if xs != nil {
		objValue := reflect.ValueOf(xs)
		return objValue.Len() == 0
	}
	return true
}

func elementsMatch(t *testing.T, listA, listB any, msgAndArgs ...any) bool {
	switch {
	case listA == nil && listB == nil:
		return true
	case listA == nil:
		return isEmpty(listB)
	case listB == nil:
		return isEmpty(listA)
	}

	aKind := reflect.TypeOf(listA).Kind()
	bKind := reflect.TypeOf(listB).Kind()

	if aKind != reflect.Array && aKind != reflect.Slice {
		return assert.Fail(t, fmt.Sprintf("%q has an unsupported type %s", listA, aKind), msgAndArgs...)
	}

	if bKind != reflect.Array && bKind != reflect.Slice {
		return assert.Fail(t, fmt.Sprintf("%q has an unsupported type %s", listB, bKind), msgAndArgs...)
	}

	aValue := reflect.ValueOf(listA)
	bValue := reflect.ValueOf(listB)

	aLen := aValue.Len()
	bLen := bValue.Len()

	if aLen != bLen {
		return assert.Fail(t, fmt.Sprintf("lengths don't match: %d != %d", aLen, bLen), msgAndArgs...)
	}

	visited := make([]bool, bLen)
	for i := range aLen {
		element := aValue.Index(i).Interface()
		found := false
		for j := range bLen {
			if visited[j] {
				continue
			}
			if cmp.Equal(bValue.Index(j).Interface(), element) {
				visited[j] = true
				found = true
				break
			}
		}
		if !found {
			return assert.Fail(t, fmt.Sprintf("element %s appears more times in %s than in %s", element, aValue, bValue), msgAndArgs...)
		}
	}

	return true
}

func TestDigitalOceanZones(t *testing.T) {
	provider := &Provider{
		client:       &mockDigitalOceanClient{},
		domainFilter: endpoint.NewDomainFilter([]string{"com"}),
	}

	zones, err := provider.zones(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	validateDigitalOceanZones(t, zones, []godo.Domain{
		{Name: "foo.com"}, {Name: "example.com"}, {Name: "bar.com"},
	})
}

func TestDigitalOceanMakeDomainEditRequest(t *testing.T) {
	// Ensure that records at the root of the zone get `@` as the name.
	r1 := makeDomainEditRequest("example.com", "example.com", endpoint.RecordTypeA,
		"1.2.3.4", defaultTTL)
	assert.Equal(t, &godo.DomainRecordEditRequest{
		Type: endpoint.RecordTypeA,
		Name: "@",
		Data: "1.2.3.4",
		TTL:  defaultTTL,
	}, r1)

	// Ensure the CNAME records have a `.` appended.
	r2 := makeDomainEditRequest("example.com", "foo.example.com", endpoint.RecordTypeCNAME,
		"bar.example.com", defaultTTL)
	assert.Equal(t, &godo.DomainRecordEditRequest{
		Type: endpoint.RecordTypeCNAME,
		Name: "foo",
		Data: "bar.example.com.",
		TTL:  defaultTTL,
	}, r2)

	// Ensure that CNAME records do not have an extra `.` appended if they already have a `.`
	r3 := makeDomainEditRequest("example.com", "foo.example.com", endpoint.RecordTypeCNAME,
		"bar.example.com.", defaultTTL)
	assert.Equal(t, &godo.DomainRecordEditRequest{
		Type: endpoint.RecordTypeCNAME,
		Name: "foo",
		Data: "bar.example.com.",
		TTL:  defaultTTL,
	}, r3)

	// Ensure that custom TTLs can be set
	customTTL := 600
	r4 := makeDomainEditRequest("example.com", "foo.example.com", endpoint.RecordTypeCNAME,
		"bar.example.com.", customTTL)
	assert.Equal(t, &godo.DomainRecordEditRequest{
		Type: endpoint.RecordTypeCNAME,
		Name: "foo",
		Data: "bar.example.com.",
		TTL:  customTTL,
	}, r4)

	// Ensure that MX records have `.` appended.
	r5 := makeDomainEditRequest("example.com", "foo.example.com", endpoint.RecordTypeMX,
		"10 mx.example.com", defaultTTL)
	assert.Equal(t, &godo.DomainRecordEditRequest{
		Type:     endpoint.RecordTypeMX,
		Name:     "foo",
		Data:     "mx.example.com.",
		Priority: 10,
		TTL:      defaultTTL,
	}, r5)

	// Ensure that MX records do not have an extra `.` appended if they already have a `.`
	r6 := makeDomainEditRequest("example.com", "foo.example.com", endpoint.RecordTypeMX,
		"10 mx.example.com.", defaultTTL)
	assert.Equal(t, &godo.DomainRecordEditRequest{
		Type:     endpoint.RecordTypeMX,
		Name:     "foo",
		Data:     "mx.example.com.",
		Priority: 10,
		TTL:      defaultTTL,
	}, r6)
}

func TestDigitalOceanApplyChanges(t *testing.T) {
	changes := &plan.Changes{}
	provider := &Provider{
		client: &mockDigitalOceanClient{},
	}
	changes.Create = []*endpoint.Endpoint{
		{DNSName: "new.ext-dns-test.bar.com", Targets: endpoint.Targets{"target"}},
		{DNSName: "new.ext-dns-test-with-ttl.bar.com", Targets: endpoint.Targets{"target"}, RecordTTL: 100},
		{DNSName: "new.ext-dns-test.unexpected.com", Targets: endpoint.Targets{"target"}},
		{DNSName: "bar.com", Targets: endpoint.Targets{"target"}},
	}
	changes.Delete = []*endpoint.Endpoint{{DNSName: "foobar.ext-dns-test.bar.com", Targets: endpoint.Targets{"target"}}}
	changes.UpdateOld = []*endpoint.Endpoint{{DNSName: "foobar.ext-dns-test.bar.de", Targets: endpoint.Targets{"target-old"}}}
	changes.UpdateNew = []*endpoint.Endpoint{{DNSName: "foobar.ext-dns-test.foo.com", Targets: endpoint.Targets{"target-new"}, RecordType: "CNAME", RecordTTL: 100}}
	err := provider.ApplyChanges(context.Background(), changes)
	if err != nil {
		t.Errorf("should not fail, %s", err)
	}
}

func TestDigitalOceanProcessCreateActions(t *testing.T) {
	recordsByDomain := map[string][]godo.DomainRecord{
		"example.com": nil,
	}

	createsByDomain := map[string][]*endpoint.Endpoint{
		"example.com": {
			endpoint.NewEndpoint("foo.example.com", endpoint.RecordTypeA, "1.2.3.4"),
			endpoint.NewEndpoint("example.com", endpoint.RecordTypeCNAME, "foo.example.com"),
			endpoint.NewEndpoint("example.com", endpoint.RecordTypeMX, "10 mx.example.com"),
			endpoint.NewEndpoint("example.com", endpoint.RecordTypeTXT, "SOME-TXT-TEXT"),
		},
	}

	var chg changes
	err := processCreateActions(recordsByDomain, createsByDomain, &chg)
	require.NoError(t, err)

	assert.Len(t, chg.Creates, 4)
	assert.Empty(t, chg.Updates)
	assert.Empty(t, chg.Deletes)

	expectedCreates := []*changeCreate{
		{
			Domain: "example.com",
			Options: &godo.DomainRecordEditRequest{
				Name: "foo",
				Type: endpoint.RecordTypeA,
				Data: "1.2.3.4",
				TTL:  defaultTTL,
			},
		},
		{
			Domain: "example.com",
			Options: &godo.DomainRecordEditRequest{
				Name: "@",
				Type: endpoint.RecordTypeCNAME,
				Data: "foo.example.com.",
				TTL:  defaultTTL,
			},
		},
		{
			Domain: "example.com",
			Options: &godo.DomainRecordEditRequest{
				Name:     "@",
				Type:     endpoint.RecordTypeMX,
				Priority: 10,
				Data:     "mx.example.com.",
				TTL:      defaultTTL,
			},
		},
		{
			Domain: "example.com",
			Options: &godo.DomainRecordEditRequest{
				Name: "@",
				Type: endpoint.RecordTypeTXT,
				Data: "SOME-TXT-TEXT",
				TTL:  defaultTTL,
			},
		},
	}

	if !elementsMatch(t, expectedCreates, chg.Creates) {
		assert.Failf(t, "diff: %s", cmp.Diff(expectedCreates, chg.Creates))
	}
}

func TestDigitalOceanProcessUpdateActions(t *testing.T) {
	recordsByDomain := map[string][]godo.DomainRecord{
		"example.com": {
			{
				ID:   1,
				Name: "foo",
				Type: endpoint.RecordTypeA,
				Data: "1.2.3.4",
				TTL:  defaultTTL,
			},
			{
				ID:   2,
				Name: "foo",
				Type: endpoint.RecordTypeA,
				Data: "5.6.7.8",
				TTL:  defaultTTL,
			},
			{
				ID:   3,
				Name: "@",
				Type: endpoint.RecordTypeCNAME,
				Data: "foo.example.com.",
				TTL:  defaultTTL,
			},
			{
				ID:       4,
				Name:     "@",
				Type:     endpoint.RecordTypeMX,
				Data:     "mx1.example.com.",
				Priority: 10,
				TTL:      defaultTTL,
			},
			{
				ID:       5,
				Name:     "@",
				Type:     endpoint.RecordTypeMX,
				Data:     "mx2.example.com.",
				Priority: 10,
				TTL:      defaultTTL,
			},
			{
				ID:   6,
				Name: "@",
				Type: endpoint.RecordTypeTXT,
				Data: "SOME_TXTX_TEXT",
				TTL:  defaultTTL,
			},
		},
	}

	updatesByDomain := map[string][]*endpoint.Endpoint{
		"example.com": {
			endpoint.NewEndpoint("foo.example.com", endpoint.RecordTypeA, "10.11.12.13"),
			endpoint.NewEndpoint("example.com", endpoint.RecordTypeCNAME, "bar.example.com"),
			endpoint.NewEndpoint("example.com", endpoint.RecordTypeMX, "10 mx3.example.com"),
			endpoint.NewEndpoint("example.com", endpoint.RecordTypeTXT, "ANOTHER-TXT"),
		},
	}

	var chg changes
	err := processUpdateActions(recordsByDomain, updatesByDomain, &chg)
	require.NoError(t, err)

	assert.Len(t, chg.Creates, 4)
	assert.Empty(t, chg.Updates)
	assert.Len(t, chg.Deletes, 6)

	expectedCreates := []*changeCreate{
		{
			Domain: "example.com",
			Options: &godo.DomainRecordEditRequest{
				Name: "foo",
				Type: endpoint.RecordTypeA,
				Data: "10.11.12.13",
				TTL:  defaultTTL,
			},
		},
		{
			Domain: "example.com",
			Options: &godo.DomainRecordEditRequest{
				Name: "@",
				Type: endpoint.RecordTypeCNAME,
				Data: "bar.example.com.",
				TTL:  defaultTTL,
			},
		},
		{
			Domain: "example.com",
			Options: &godo.DomainRecordEditRequest{
				Name:     "@",
				Type:     endpoint.RecordTypeMX,
				Data:     "mx3.example.com.",
				Priority: 10,
				TTL:      defaultTTL,
			},
		},
		{
			Domain: "example.com",
			Options: &godo.DomainRecordEditRequest{
				Name: "@",
				Type: endpoint.RecordTypeTXT,
				Data: "ANOTHER-TXT",
				TTL:  defaultTTL,
			},
		},
	}

	if !elementsMatch(t, expectedCreates, chg.Creates) {
		assert.Failf(t, "diff: %s", cmp.Diff(expectedCreates, chg.Creates))
	}

	expectedDeletes := []*changeDelete{
		{Domain: "example.com", RecordID: 1},
		{Domain: "example.com", RecordID: 2},
		{Domain: "example.com", RecordID: 3},
		{Domain: "example.com", RecordID: 4},
		{Domain: "example.com", RecordID: 5},
		{Domain: "example.com", RecordID: 6},
	}

	if !elementsMatch(t, expectedDeletes, chg.Deletes) {
		assert.Failf(t, "diff: %s", cmp.Diff(expectedDeletes, chg.Deletes))
	}
}

func TestDigitalOceanProcessDeleteActions(t *testing.T) {
	recordsByDomain := map[string][]godo.DomainRecord{
		"example.com": {
			{
				ID:   1,
				Name: "foo",
				Type: endpoint.RecordTypeA,
				Data: "1.2.3.4",
				TTL:  defaultTTL,
			},
			// This record will not be deleted because it represents a target not specified to be deleted.
			{
				ID:   2,
				Name: "foo",
				Type: endpoint.RecordTypeA,
				Data: "5.6.7.8",
				TTL:  defaultTTL,
			},
			{
				ID:   3,
				Name: "@",
				Type: endpoint.RecordTypeCNAME,
				Data: "foo.example.com.",
				TTL:  defaultTTL,
			},
		},
	}

	deletesByDomain := map[string][]*endpoint.Endpoint{
		"example.com": {
			endpoint.NewEndpoint("foo.example.com", endpoint.RecordTypeA, "1.2.3.4"),
			endpoint.NewEndpoint("example.com", endpoint.RecordTypeCNAME, "foo.example.com"),
		},
	}

	var chg changes
	err := processDeleteActions(recordsByDomain, deletesByDomain, &chg)
	require.NoError(t, err)

	assert.Empty(t, chg.Creates)
	assert.Empty(t, chg.Updates)
	assert.Len(t, chg.Deletes, 2)

	expectedDeletes := []*changeDelete{
		{Domain: "example.com", RecordID: 1},
		{Domain: "example.com", RecordID: 3},
	}

	if !elementsMatch(t, expectedDeletes, chg.Deletes) {
		assert.Failf(t, "diff: %s", cmp.Diff(expectedDeletes, chg.Deletes))
	}
}

func TestDigitalOceanGetMatchingDomainRecords(t *testing.T) {
	records := []godo.DomainRecord{
		{
			ID:   1,
			Name: "foo",
			Type: endpoint.RecordTypeCNAME,
			Data: "baz.org.",
		},
		{
			ID:   2,
			Name: "baz",
			Type: endpoint.RecordTypeA,
			Data: "1.2.3.4",
		},
		{
			ID:   3,
			Name: "baz",
			Type: endpoint.RecordTypeA,
			Data: "5.6.7.8",
		},
		{
			ID:   4,
			Name: "@",
			Type: endpoint.RecordTypeA,
			Data: "9.10.11.12",
		},
		{
			ID:       5,
			Name:     "@",
			Type:     endpoint.RecordTypeMX,
			Priority: 10,
			Data:     "mx1.foo.com.",
		},
		{
			ID:       6,
			Name:     "@",
			Type:     endpoint.RecordTypeMX,
			Priority: 10,
			Data:     "mx2.foo.com.",
		},
		{
			ID:   7,
			Name: "@",
			Type: endpoint.RecordTypeTXT,
			Data: "MYTXT",
		},
	}

	ep1 := endpoint.NewEndpoint("foo.com", endpoint.RecordTypeCNAME)
	assert.Len(t, getMatchingDomainRecords(records, "com", ep1), 1)

	ep2 := endpoint.NewEndpoint("foo.com", endpoint.RecordTypeA)
	assert.Empty(t, getMatchingDomainRecords(records, "com", ep2))

	ep3 := endpoint.NewEndpoint("baz.org", endpoint.RecordTypeA)
	r := getMatchingDomainRecords(records, "org", ep3)
	assert.Len(t, r, 2)
	assert.ElementsMatch(t, r, []godo.DomainRecord{
		{
			ID:   2,
			Name: "baz",
			Type: endpoint.RecordTypeA,
			Data: "1.2.3.4",
		},
		{
			ID:   3,
			Name: "baz",
			Type: endpoint.RecordTypeA,
			Data: "5.6.7.8",
		},
	})

	ep4 := endpoint.NewEndpoint("example.com", endpoint.RecordTypeA)
	r2 := getMatchingDomainRecords(records, "example.com", ep4)
	assert.Len(t, r2, 1)
	assert.Equal(t, "9.10.11.12", r2[0].Data)

	ep5 := endpoint.NewEndpoint("example.com", endpoint.RecordTypeMX)
	r3 := getMatchingDomainRecords(records, "example.com", ep5)
	assert.Len(t, r3, 2)
	assert.Equal(t, "mx1.foo.com.", r3[0].Data)
	assert.Equal(t, "mx2.foo.com.", r3[1].Data)

	ep6 := endpoint.NewEndpoint("example.com", endpoint.RecordTypeTXT)
	r4 := getMatchingDomainRecords(records, "example.com", ep6)
	assert.Len(t, r4, 1)
	assert.Equal(t, "MYTXT", r4[0].Data)
}

func validateDigitalOceanZones(t *testing.T, zones []godo.Domain, expected []godo.Domain) {
	require.Len(t, zones, len(expected))

	for i, zone := range zones {
		assert.Equal(t, expected[i].Name, zone.Name)
	}
}

func TestDigitalOceanRecord(t *testing.T) {
	provider := &Provider{
		client: &mockDigitalOceanClient{},
	}

	records, err := provider.fetchRecords(context.Background(), "example.com", 1)
	if err != nil {
		t.Fatal(err)
	}
	expected := []godo.DomainRecord{{ID: 1, Name: "new", Type: "CNAME"}}
	require.Len(t, records, len(expected))
	for i, record := range records {
		assert.Equal(t, expected[i].Name, record.Name)
	}
}

func TestDigitalOceanAllRecords(t *testing.T) {
	provider := &Provider{
		client: &mockDigitalOceanClient{},
	}
	ctx := context.Background()

	records, err := provider.Records(ctx)
	if err != nil {
		t.Errorf("should not fail, %s", err)
	}
	require.Len(t, records, 7)

	provider.client = &mockDigitalOceanRecordsFail{}
	_, err = provider.Records(ctx)
	if err == nil {
		t.Errorf("expected to fail, %s", err)
	}
}

func TestDigitalOceanMergeRecordsByNameType(t *testing.T) {
	xs := []*endpoint.Endpoint{
		endpoint.NewEndpoint("foo.example.com", "A", "1.2.3.4"),
		endpoint.NewEndpoint("bar.example.com", "A", "1.2.3.4"),
		endpoint.NewEndpoint("foo.example.com", "A", "5.6.7.8"),
		endpoint.NewEndpoint("foo.example.com", "CNAME", "somewhere.out.there.com"),
		endpoint.NewEndpoint("bar.example.com", "MX", "10 bar.mx1.com"),
		endpoint.NewEndpoint("bar.example.com", "MX", "10 bar.mx2.com"),
		endpoint.NewEndpoint("foo.example.com", "TXT", "txtone"),
		endpoint.NewEndpoint("foo.example.com", "TXT", "txttwo"),
	}

	merged := mergeEndpointsByNameType(xs)

	assert.Len(t, merged, 5)
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].DNSName != merged[j].DNSName {
			return merged[i].DNSName < merged[j].DNSName
		}
		return merged[i].RecordType < merged[j].RecordType
	})
	assert.Equal(t, "bar.example.com", merged[0].DNSName)
	assert.Equal(t, "A", merged[0].RecordType)
	assert.Len(t, merged[0].Targets, 1)
	assert.Equal(t, "1.2.3.4", merged[0].Targets[0])
	assert.Equal(t, "MX", merged[1].RecordType)
	assert.Len(t, merged[1].Targets, 2)
	assert.ElementsMatch(t, []string{"10 bar.mx1.com", "10 bar.mx2.com"}, merged[1].Targets)

	assert.Equal(t, "foo.example.com", merged[2].DNSName)
	assert.Equal(t, "A", merged[2].RecordType)
	assert.Len(t, merged[2].Targets, 2)
	assert.ElementsMatch(t, []string{"1.2.3.4", "5.6.7.8"}, merged[2].Targets)

	assert.Equal(t, "foo.example.com", merged[3].DNSName)
	assert.Equal(t, "CNAME", merged[3].RecordType)
	assert.Len(t, merged[3].Targets, 1)
	assert.Equal(t, "somewhere.out.there.com", merged[3].Targets[0])

	assert.Equal(t, "foo.example.com", merged[4].DNSName)
	assert.Equal(t, "TXT", merged[4].RecordType)
	assert.Len(t, merged[4].Targets, 2)
	assert.ElementsMatch(t, []string{"txtone", "txttwo"}, merged[4].Targets)
}

// Test for trailing dot normalization fix
func TestDigitalOceanTrailingDotNormalization(t *testing.T) {
	// This test verifies that CNAME records with trailing dots in DO API
	// are correctly matched with endpoints without trailing dots
	recordsByDomain := map[string][]godo.DomainRecord{
		"example.com": {
			{
				ID:   1,
				Name: "www",
				Type: endpoint.RecordTypeCNAME,
				Data: "target.example.com.", // DO API returns with trailing dot
				TTL:  defaultTTL,
			},
		},
	}

	// Update with endpoint that doesn't have trailing dot
	updatesByDomain := map[string][]*endpoint.Endpoint{
		"example.com": {
			endpoint.NewEndpoint("www.example.com", endpoint.RecordTypeCNAME, "target.example.com"), // No trailing dot
		},
	}

	var chg changes
	err := processUpdateActions(recordsByDomain, updatesByDomain, &chg)
	require.NoError(t, err)

	// Should update existing record, not create + delete
	assert.Len(t, chg.Updates, 1)
	assert.Empty(t, chg.Creates)
	assert.Empty(t, chg.Deletes)
}

// Test for trailing dot normalization in delete actions
func TestDigitalOceanTrailingDotNormalizationDelete(t *testing.T) {
	recordsByDomain := map[string][]godo.DomainRecord{
		"example.com": {
			{
				ID:   1,
				Name: "www",
				Type: endpoint.RecordTypeCNAME,
				Data: "target.example.com.", // DO API returns with trailing dot
				TTL:  defaultTTL,
			},
		},
	}

	// Delete with endpoint that doesn't have trailing dot
	deletesByDomain := map[string][]*endpoint.Endpoint{
		"example.com": {
			endpoint.NewEndpoint("www.example.com", endpoint.RecordTypeCNAME, "target.example.com"), // No trailing dot
		},
	}

	var chg changes
	err := processDeleteActions(recordsByDomain, deletesByDomain, &chg)
	require.NoError(t, err)

	// Should delete the record
	assert.Len(t, chg.Deletes, 1)
	assert.Equal(t, 1, chg.Deletes[0].RecordID)
}
