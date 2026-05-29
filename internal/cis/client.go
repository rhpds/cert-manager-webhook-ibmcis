package cis

import (
	"context"
	"fmt"

	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/networking-go-sdk/dnsrecordsv1"
	"github.com/IBM/networking-go-sdk/zonesv1"
)

type CISClient interface {
	CreateTXTRecord(ctx context.Context, crn, zoneID, name, content string) error
	DeleteTXTRecord(ctx context.Context, crn, zoneID, recordID string) error
	ListTXTRecords(ctx context.Context, crn, zoneID, name string) ([]DNSRecord, error)
	ListZones(ctx context.Context, crn string) ([]Zone, error)
}

type DNSRecord struct {
	ID      string
	Name    string
	Content string
}

type Zone struct {
	ID   string
	Name string
}

type client struct {
	apiKey string
}

func NewClient(apiKey string) CISClient {
	return &client{apiKey: apiKey}
}

func (c *client) authenticator() *core.IamAuthenticator {
	return &core.IamAuthenticator{ApiKey: c.apiKey}
}

func (c *client) ListZones(ctx context.Context, crn string) ([]Zone, error) {
	svc, err := zonesv1.NewZonesV1(&zonesv1.ZonesV1Options{
		Authenticator: c.authenticator(),
		Crn:           core.StringPtr(crn),
	})
	if err != nil {
		return nil, fmt.Errorf("creating zones service: %w", err)
	}

	resp, _, err := svc.ListZonesWithContext(ctx, &zonesv1.ListZonesOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing zones for CRN %s: %w", crn, err)
	}

	zones := make([]Zone, 0, len(resp.Result))
	for _, z := range resp.Result {
		zones = append(zones, Zone{
			ID:   *z.ID,
			Name: *z.Name,
		})
	}
	return zones, nil
}

func (c *client) ListTXTRecords(ctx context.Context, crn, zoneID, name string) ([]DNSRecord, error) {
	svc, err := dnsrecordsv1.NewDnsRecordsV1(&dnsrecordsv1.DnsRecordsV1Options{
		Authenticator:  c.authenticator(),
		Crn:            core.StringPtr(crn),
		ZoneIdentifier: core.StringPtr(zoneID),
	})
	if err != nil {
		return nil, fmt.Errorf("creating dns records service: %w", err)
	}

	txtType := "TXT"
	resp, _, err := svc.ListAllDnsRecordsWithContext(ctx, &dnsrecordsv1.ListAllDnsRecordsOptions{
		Type: &txtType,
		Name: &name,
	})
	if err != nil {
		return nil, fmt.Errorf("listing TXT records: %w", err)
	}

	records := make([]DNSRecord, 0, len(resp.Result))
	for _, r := range resp.Result {
		records = append(records, DNSRecord{
			ID:      *r.ID,
			Name:    *r.Name,
			Content: *r.Content,
		})
	}
	return records, nil
}

func (c *client) CreateTXTRecord(ctx context.Context, crn, zoneID, name, content string) error {
	svc, err := dnsrecordsv1.NewDnsRecordsV1(&dnsrecordsv1.DnsRecordsV1Options{
		Authenticator:  c.authenticator(),
		Crn:            core.StringPtr(crn),
		ZoneIdentifier: core.StringPtr(zoneID),
	})
	if err != nil {
		return fmt.Errorf("creating dns records service: %w", err)
	}

	txtType := "TXT"
	_, _, err = svc.CreateDnsRecordWithContext(ctx, &dnsrecordsv1.CreateDnsRecordOptions{
		Type:    &txtType,
		Name:    &name,
		Content: &content,
	})
	if err != nil {
		return fmt.Errorf("creating TXT record %s: %w", name, err)
	}
	return nil
}

func (c *client) DeleteTXTRecord(ctx context.Context, crn, zoneID, recordID string) error {
	svc, err := dnsrecordsv1.NewDnsRecordsV1(&dnsrecordsv1.DnsRecordsV1Options{
		Authenticator:  c.authenticator(),
		Crn:            core.StringPtr(crn),
		ZoneIdentifier: core.StringPtr(zoneID),
	})
	if err != nil {
		return fmt.Errorf("creating dns records service: %w", err)
	}

	_, _, err = svc.DeleteDnsRecordWithContext(ctx, &dnsrecordsv1.DeleteDnsRecordOptions{
		DnsrecordIdentifier: &recordID,
	})
	if err != nil {
		return fmt.Errorf("deleting TXT record %s: %w", recordID, err)
	}
	return nil
}
