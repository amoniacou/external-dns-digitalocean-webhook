package digitalocean

import (
	"context"
	"fmt"
	"strings"

	"github.com/digitalocean/godo"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

const (
	defaultTTL = 300
)

// Provider implements the DNS provider interface for DigitalOcean
type Provider struct {
	provider.BaseProvider
	client       godo.DomainsService
	domainFilter *endpoint.DomainFilter
	apiPageSize  int
	dryRun       bool
}

type changeCreate struct {
	Domain  string
	Options *godo.DomainRecordEditRequest
}

type changeUpdate struct {
	Domain       string
	DomainRecord godo.DomainRecord
	Options      *godo.DomainRecordEditRequest
}

type changeDelete struct {
	Domain   string
	RecordID int
}

type changes struct {
	Creates []*changeCreate
	Updates []*changeUpdate
	Deletes []*changeDelete
}

func (c *changes) Empty() bool {
	return len(c.Creates) == 0 && len(c.Updates) == 0 && len(c.Deletes) == 0
}

// NewProvider creates a new DigitalOcean DNS provider
func NewProvider(cfg *Config) (*Provider, error) {
	oauthClient := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: cfg.APIToken,
	}))
	
	// Wrap transport for metrics
	oauthClient.Transport = &metricsRoundTripper{
		base: oauthClient.Transport,
	}

	// Configure retry with exponential backoff for rate limit (429) and server errors (5xx)
	retryConfig := godo.RetryConfig{
		RetryMax:     cfg.HTTPRetryMax,
		RetryWaitMin: godo.PtrTo(cfg.HTTPRetryWaitMin.Seconds()),
		RetryWaitMax: godo.PtrTo(cfg.HTTPRetryWaitMax.Seconds()),
	}

	client, err := godo.New(
		oauthClient,
		godo.SetUserAgent("external-dns-digitalocean-webhook"),
		godo.WithRetryAndBackoffs(retryConfig),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create DigitalOcean client: %w", err)
	}

	log.Infof("DigitalOcean provider configured with retry: max=%d, waitMin=%s, waitMax=%s",
		cfg.HTTPRetryMax, cfg.HTTPRetryWaitMin, cfg.HTTPRetryWaitMax)

	return &Provider{
		client:       client.Domains,
		domainFilter: endpoint.NewDomainFilter(cfg.DomainFilter),
		apiPageSize:  cfg.APIPageSize,
		dryRun:       cfg.DryRun,
	}, nil
}

// Records returns the list of DNS records
func (p *Provider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	zones, err := p.zones(ctx)
	if err != nil {
		return nil, err
	}

	var endpoints []*endpoint.Endpoint
	for _, zone := range zones {
		records, err := p.fetchRecords(ctx, zone.Name)
		if err != nil {
			return nil, err
		}

		for _, r := range records {
			if p.SupportedRecordType(r.Type) {
				name := r.Name + "." + zone.Name
				data := r.Data

				if r.Name == "@" {
					name = zone.Name
				}

				if r.Type == endpoint.RecordTypeMX {
					data = fmt.Sprintf("%d %s", r.Priority, r.Data)
				}

				ep := endpoint.NewEndpointWithTTL(name, r.Type, endpoint.TTL(r.TTL), data)
				endpoints = append(endpoints, ep)
			}
		}
	}

	endpoints = mergeEndpointsByNameType(endpoints)

	log.WithFields(log.Fields{
		"endpoints": len(endpoints),
	}).Debug("Endpoints generated from DigitalOcean DNS")

	return endpoints, nil
}

// ApplyChanges applies DNS changes to DigitalOcean
func (p *Provider) ApplyChanges(ctx context.Context, planChanges *plan.Changes) error {
	recordsByDomain, zoneNameIDMapper, err := p.getRecordsByDomain(ctx)
	if err != nil {
		return err
	}

	createsByDomain := endpointsByZone(zoneNameIDMapper, planChanges.Create)
	updatesByDomain := endpointsByZone(zoneNameIDMapper, planChanges.UpdateNew)
	deletesByDomain := endpointsByZone(zoneNameIDMapper, planChanges.Delete)

	var chg changes

	if err := processCreateActions(recordsByDomain, createsByDomain, &chg); err != nil {
		return err
	}

	if err := processUpdateActions(recordsByDomain, updatesByDomain, &chg); err != nil {
		return err
	}

	if err := processDeleteActions(recordsByDomain, deletesByDomain, &chg); err != nil {
		return err
	}

	return p.submitChanges(ctx, &chg)
}

// SupportedRecordType returns true if the record type is supported
func (p *Provider) SupportedRecordType(recordType string) bool {
	switch recordType {
	case "MX":
		return true
	default:
		return provider.SupportedRecordType(recordType)
	}
}

// GetDomainFilter returns the domain filter
func (p *Provider) GetDomainFilter() endpoint.DomainFilterInterface {
	return p.domainFilter
}

func (p *Provider) zones(ctx context.Context) ([]godo.Domain, error) {
	var result []godo.Domain

	allZones, err := p.fetchZones(ctx)
	if err != nil {
		return nil, err
	}

	for _, zone := range allZones {
		if p.domainFilter.Match(zone.Name) {
			result = append(result, zone)
		}
	}

	return result, nil
}

func (p *Provider) fetchZones(ctx context.Context) ([]godo.Domain, error) {
	var allZones []godo.Domain
	listOptions := &godo.ListOptions{PerPage: p.apiPageSize}

	for {
		zones, resp, err := p.client.List(ctx, listOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list zones: %w", err)
		}
		allZones = append(allZones, zones...)

		if resp == nil || resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, err
		}
		listOptions.Page = page + 1
	}

	return allZones, nil
}

func (p *Provider) fetchRecords(ctx context.Context, zoneName string) ([]godo.DomainRecord, error) {
	var allRecords []godo.DomainRecord
	listOptions := &godo.ListOptions{PerPage: p.apiPageSize}

	for {
		records, resp, err := p.client.Records(ctx, zoneName, listOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list records for zone %s: %w", zoneName, err)
		}
		allRecords = append(allRecords, records...)

		if resp == nil || resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, err
		}
		listOptions.Page = page + 1
	}

	return allRecords, nil
}

func (p *Provider) getRecordsByDomain(ctx context.Context) (map[string][]godo.DomainRecord, provider.ZoneIDName, error) {
	recordsByDomain := map[string][]godo.DomainRecord{}

	zones, err := p.zones(ctx)
	if err != nil {
		return nil, nil, err
	}

	zoneNameIDMapper := provider.ZoneIDName{}
	for _, z := range zones {
		zoneNameIDMapper.Add(z.Name, z.Name)
	}

	for _, zone := range zones {
		records, err := p.fetchRecords(ctx, zone.Name)
		if err != nil {
			return nil, nil, err
		}
		recordsByDomain[zone.Name] = append(recordsByDomain[zone.Name], records...)
	}

	return recordsByDomain, zoneNameIDMapper, nil
}

func (p *Provider) submitChanges(ctx context.Context, chg *changes) error {
	if chg.Empty() {
		return nil
	}

	var errs []error

	for _, c := range chg.Creates {
		logFields := log.Fields{
			"domain":     c.Domain,
			"dnsName":    c.Options.Name,
			"recordType": c.Options.Type,
			"data":       c.Options.Data,
			"ttl":        c.Options.TTL,
		}
		if c.Options.Type == endpoint.RecordTypeMX {
			logFields["priority"] = c.Options.Priority
		}
		log.WithFields(logFields).Info("Creating domain record")

		if p.dryRun {
			continue
		}

		_, _, err := p.client.CreateRecord(ctx, c.Domain, c.Options)
		if err != nil {
			log.WithFields(logFields).WithError(err).Error("Failed to create record")
			errs = append(errs, fmt.Errorf("create %s.%s: %w", c.Options.Name, c.Domain, err))
		}
	}

	for _, u := range chg.Updates {
		logFields := log.Fields{
			"domain":     u.Domain,
			"dnsName":    u.Options.Name,
			"recordType": u.Options.Type,
			"data":       u.Options.Data,
			"ttl":        u.Options.TTL,
			"recordID":   u.DomainRecord.ID,
		}
		if u.Options.Type == endpoint.RecordTypeMX {
			logFields["priority"] = u.Options.Priority
		}
		log.WithFields(logFields).Info("Updating domain record")

		if p.dryRun {
			continue
		}

		_, _, err := p.client.EditRecord(ctx, u.Domain, u.DomainRecord.ID, u.Options)
		if err != nil {
			log.WithFields(logFields).WithError(err).Error("Failed to update record")
			errs = append(errs, fmt.Errorf("update %s.%s: %w", u.Options.Name, u.Domain, err))
		}
	}

	for _, d := range chg.Deletes {
		logFields := log.Fields{
			"domain":   d.Domain,
			"recordID": d.RecordID,
		}
		log.WithFields(logFields).Info("Deleting domain record")

		if p.dryRun {
			continue
		}

		_, err := p.client.DeleteRecord(ctx, d.Domain, d.RecordID)
		if err != nil {
			log.WithFields(logFields).WithError(err).Error("Failed to delete record")
			errs = append(errs, fmt.Errorf("delete record %d in %s: %w", d.RecordID, d.Domain, err))
		}
	}

	if len(errs) > 0 {
		// Return as SoftError so external-dns will retry on next cycle
		return provider.NewSoftError(fmt.Errorf("some changes failed (%d errors): %v", len(errs), errs))
	}

	return nil
}

// Helper functions

func mergeEndpointsByNameType(endpoints []*endpoint.Endpoint) []*endpoint.Endpoint {
	endpointsByNameType := map[string][]*endpoint.Endpoint{}

	for _, e := range endpoints {
		key := fmt.Sprintf("%s-%s", e.DNSName, e.RecordType)
		endpointsByNameType[key] = append(endpointsByNameType[key], e)
	}

	if len(endpointsByNameType) == len(endpoints) {
		return endpoints
	}

	var result []*endpoint.Endpoint
	for _, eps := range endpointsByNameType {
		dnsName := eps[0].DNSName
		recordType := eps[0].RecordType

		targets := make([]string, len(eps))
		for i, e := range eps {
			targets[i] = e.Targets[0]
		}

		e := endpoint.NewEndpoint(dnsName, recordType, targets...)
		result = append(result, e)
	}

	return result
}

func endpointsByZone(zoneNameIDMapper provider.ZoneIDName, endpoints []*endpoint.Endpoint) map[string][]*endpoint.Endpoint {
	result := make(map[string][]*endpoint.Endpoint)

	for _, ep := range endpoints {
		zoneID, _ := zoneNameIDMapper.FindZone(ep.DNSName)
		if zoneID == "" {
			log.Debugf("Skipping record %s because no hosted zone matching record DNS Name was detected", ep.DNSName)
			continue
		}
		result[zoneID] = append(result[zoneID], ep)
	}

	return result
}

func getMatchingDomainRecords(records []godo.DomainRecord, domain string, ep *endpoint.Endpoint) []godo.DomainRecord {
	var name string
	if ep.DNSName != domain {
		name = strings.TrimSuffix(ep.DNSName, "."+domain)
	} else {
		name = "@"
	}

	var result []godo.DomainRecord
	for _, r := range records {
		if r.Name == name && r.Type == ep.RecordType {
			result = append(result, r)
		}
	}
	return result
}

func getTTLFromEndpoint(ep *endpoint.Endpoint) int {
	if ep.RecordTTL.IsConfigured() {
		return int(ep.RecordTTL)
	}
	return defaultTTL
}

func makeDomainEditRequest(domain, name, recordType, data string, ttl int) *godo.DomainRecordEditRequest {
	adjustedName := strings.TrimSuffix(name, "."+domain)

	if adjustedName == domain {
		adjustedName = "@"
	}

	if (recordType == endpoint.RecordTypeCNAME || recordType == endpoint.RecordTypeMX) && !strings.HasSuffix(data, ".") {
		data += "."
	}

	request := &godo.DomainRecordEditRequest{
		Name: adjustedName,
		Type: recordType,
		Data: data,
		TTL:  ttl,
	}

	if recordType == endpoint.RecordTypeMX {
		mxRecord, err := endpoint.NewMXRecord(data)
		if err != nil {
			log.WithFields(log.Fields{
				"domain":     domain,
				"dnsName":    name,
				"recordType": recordType,
				"data":       data,
			}).Warn("Unable to parse MX target")
			return request
		}
		request.Priority = int(*mxRecord.GetPriority())
		request.Data = provider.EnsureTrailingDot(*mxRecord.GetHost())
	}

	return request
}

func processCreateActions(recordsByDomain map[string][]godo.DomainRecord, createsByDomain map[string][]*endpoint.Endpoint, chg *changes) error {
	for domain, endpoints := range createsByDomain {
		if len(endpoints) == 0 {
			continue
		}

		records := recordsByDomain[domain]

		for _, ep := range endpoints {
			matchingRecords := getMatchingDomainRecords(records, domain, ep)
			if len(matchingRecords) > 0 {
				log.WithFields(log.Fields{
					"domain":     domain,
					"dnsName":    ep.DNSName,
					"recordType": ep.RecordType,
				}).Warn("Preexisting records exist which should not exist for creation actions")
			}

			ttl := getTTLFromEndpoint(ep)

			for _, target := range ep.Targets {
				chg.Creates = append(chg.Creates, &changeCreate{
					Domain:  domain,
					Options: makeDomainEditRequest(domain, ep.DNSName, ep.RecordType, target, ttl),
				})
			}
		}
	}

	return nil
}

func processUpdateActions(recordsByDomain map[string][]godo.DomainRecord, updatesByDomain map[string][]*endpoint.Endpoint, chg *changes) error {
	for domain, updates := range updatesByDomain {
		if len(updates) == 0 {
			continue
		}

		records := recordsByDomain[domain]

		for _, ep := range updates {
			matchingRecords := getMatchingDomainRecords(records, domain, ep)

			if len(matchingRecords) == 0 {
				log.WithFields(log.Fields{
					"domain":     domain,
					"dnsName":    ep.DNSName,
					"recordType": ep.RecordType,
				}).Warn("Planning an update but no existing records found")
			}

			matchingRecordsByTarget := map[string]godo.DomainRecord{}
			for _, r := range matchingRecords {
				// Normalize key for CNAME/MX records (remove trailing dot)
				key := r.Data
				if ep.RecordType == endpoint.RecordTypeCNAME || ep.RecordType == endpoint.RecordTypeMX {
					key = strings.TrimSuffix(r.Data, ".")
				}
				matchingRecordsByTarget[key] = r
			}

			ttl := getTTLFromEndpoint(ep)

			for _, target := range ep.Targets {
				// Normalize lookup key for CNAME/MX
				lookupKey := target
				if ep.RecordType == endpoint.RecordTypeCNAME || ep.RecordType == endpoint.RecordTypeMX {
					lookupKey = strings.TrimSuffix(target, ".")
				}

				if record, ok := matchingRecordsByTarget[lookupKey]; ok {
					chg.Updates = append(chg.Updates, &changeUpdate{
						Domain:       domain,
						DomainRecord: record,
						Options:      makeDomainEditRequest(domain, ep.DNSName, ep.RecordType, target, ttl),
					})
					delete(matchingRecordsByTarget, lookupKey)
				} else {
					chg.Creates = append(chg.Creates, &changeCreate{
						Domain:  domain,
						Options: makeDomainEditRequest(domain, ep.DNSName, ep.RecordType, target, ttl),
					})
				}
			}

			for _, record := range matchingRecordsByTarget {
				chg.Deletes = append(chg.Deletes, &changeDelete{
					Domain:   domain,
					RecordID: record.ID,
				})
			}
		}
	}

	return nil
}

func processDeleteActions(recordsByDomain map[string][]godo.DomainRecord, deletesByDomain map[string][]*endpoint.Endpoint, chg *changes) error {
	for domain, deletes := range deletesByDomain {
		if len(deletes) == 0 {
			continue
		}

		records := recordsByDomain[domain]

		for _, ep := range deletes {
			matchingRecords := getMatchingDomainRecords(records, domain, ep)

			if len(matchingRecords) == 0 {
				log.WithFields(log.Fields{
					"domain":     domain,
					"dnsName":    ep.DNSName,
					"recordType": ep.RecordType,
				}).Warn("Records to delete not found")
			}

			for _, record := range matchingRecords {
				doDelete := false
				for _, t := range ep.Targets {
					v1 := t
					v2 := record.Data
					if ep.RecordType == endpoint.RecordTypeCNAME || ep.RecordType == endpoint.RecordTypeMX {
						v1 = strings.TrimSuffix(t, ".")
						v2 = strings.TrimSuffix(record.Data, ".")
					}
					if v1 == v2 {
						doDelete = true
					}
				}

				if doDelete {
					chg.Deletes = append(chg.Deletes, &changeDelete{
						Domain:   domain,
						RecordID: record.ID,
					})
				}
			}
		}
	}

	return nil
}
