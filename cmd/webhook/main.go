package main

import (
	"log/slog"
	"os"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
	"github.com/rhpds/cert-manager-webhook-ibmcis/internal/cis"
	"github.com/rhpds/cert-manager-webhook-ibmcis/internal/solver"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	groupName := os.Getenv("GROUP_NAME")
	if groupName == "" {
		groupName = "acme.borup.work"
	}

	apiKey := os.Getenv("IC_API_KEY")
	if apiKey == "" {
		panic("IC_API_KEY must be specified")
	}

	slog.Info("starting webhook", "groupName", groupName)

	cisClient := cis.NewClient(apiKey)
	cmd.RunWebhookServer(groupName, solver.New(cisClient))
}
