package solver

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/rhpds/cert-manager-webhook-ibmcis/internal/cis"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Solver struct {
	client    *kubernetes.Clientset
	cisClient cis.CISClient
}

func New(cisClient cis.CISClient) *Solver {
	return &Solver{cisClient: cisClient}
}

func (s *Solver) Name() string {
	return "ibmcis"
}

func (s *Solver) Initialize(kubeClientConfig *rest.Config, _ <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}
	s.client = cl
	slog.Info("webhook initialized")
	return nil
}

func (s *Solver) Present(ch *v1alpha1.ChallengeRequest) error {
	slog.Info("presenting DNS01 challenge",
		"namespace", ch.ResourceNamespace,
		"zone", ch.ResolvedZone,
		"fqdn", ch.ResolvedFQDN,
	)

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fqdn := normalizeFQDN(ch.ResolvedFQDN)

	if !strings.HasPrefix(fqdn, "_acme-challenge.") {
		slog.Warn("FQDN does not start with _acme-challenge", "fqdn", fqdn)
	}

	ctx := context.Background()
	crn, zoneID, err := s.findMatchingZone(ctx, fqdn, cfg.CisCRNs)
	if err != nil {
		return err
	}

	if err := s.cisClient.CreateTXTRecord(ctx, crn, zoneID, fqdn, ch.Key); err != nil {
		return fmt.Errorf("creating DNS record: %w", err)
	}

	slog.Info("DNS01 challenge presented", "fqdn", fqdn)
	return nil
}

func (s *Solver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	slog.Info("cleaning up DNS01 challenge",
		"namespace", ch.ResourceNamespace,
		"zone", ch.ResolvedZone,
		"fqdn", ch.ResolvedFQDN,
	)

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	fqdn := normalizeFQDN(ch.ResolvedFQDN)
	ctx := context.Background()

	crn, zoneID, err := s.findMatchingZone(ctx, fqdn, cfg.CisCRNs)
	if err != nil {
		return err
	}

	records, err := s.cisClient.ListTXTRecords(ctx, crn, zoneID, fqdn)
	if err != nil {
		return fmt.Errorf("listing DNS records: %w", err)
	}

	for _, rec := range records {
		if rec.Content != ch.Key {
			continue
		}
		slog.Info("deleting challenge record", "id", rec.ID, "fqdn", fqdn)
		if err := s.cisClient.DeleteTXTRecord(ctx, crn, zoneID, rec.ID); err != nil {
			return fmt.Errorf("deleting DNS record %s: %w", rec.ID, err)
		}
	}

	return nil
}

func (s *Solver) findMatchingZone(ctx context.Context, fqdn string, crns []string) (string, string, error) {
	for _, crn := range crns {
		zones, err := s.cisClient.ListZones(ctx, crn)
		if err != nil {
			slog.Error("failed to list zones", "crn", crn, "error", err)
			continue
		}
		for _, zone := range zones {
			if strings.HasSuffix(fqdn, zone.Name+".") {
				return crn, zone.ID, nil
			}
		}
	}
	return "", "", fmt.Errorf("no matching zone found for %s", fqdn)
}

func normalizeFQDN(fqdn string) string {
	if !strings.HasSuffix(fqdn, ".") {
		return fqdn + "."
	}
	return fqdn
}
