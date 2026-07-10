package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	msgraphsdkgo "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/purpose-robot/meraki-entra-sync/internal/azure"
	"github.com/purpose-robot/meraki-entra-sync/internal/config"
	"github.com/purpose-robot/meraki-entra-sync/internal/meraki"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	config, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	graphClient, err := azure.NewGraphClient(config.Azure)
	if err != nil {
		log.Fatalf("failed to create Azure graph client: %v", err)
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		if err := runTicker(ctx, config, graphClient); err != nil {
			log.Printf("tick failed: %v", err)
		}

		select {
		case <-ctx.Done():
			log.Println("received shutdown signal; stopping ticker")
			return

		case <-ticker.C:
		}
	}
}

func buildDesiredIPSet(statuses meraki.ApplianceUplinkStatuses) map[string]azure.CIDRSet {
	desired := make(map[string]azure.CIDRSet)

	for _, appliance := range statuses {
		ipSet := desired[appliance.NetworkID]
		if ipSet == nil {
			ipSet = make(azure.CIDRSet)
			desired[appliance.NetworkID] = ipSet
		}

		for _, uplink := range appliance.Uplinks {
			if uplink.Status != "ready" && uplink.Status != "active" {
				continue
			}

			addr := uplink.PublicIP.Unmap()
			if !addr.IsValid() {
				continue
			}

			ipSet[netip.PrefixFrom(addr, addr.BitLen()).String()] = struct{}{}
		}
	}

	return desired
}

func runTicker(ctx context.Context, config config.Config, graphClient *msgraphsdkgo.GraphServiceClient) error {
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()

	networks, err := meraki.GetNetworks(ctx, config.Meraki)
	if err != nil {
		return err
	}

	if len(networks) == 0 {
		log.Printf("no Meraki networks tagged with %v found; skipping this tick", config.Meraki.AllowedNetworkTags)
		return nil
	}

	networkIDs := make([]string, 0, len(networks))
	nameByNetworkID := make(map[string]string, len(networks))

	for _, network := range networks {
		nameByNetworkID[network.ID] = network.Name
		networkIDs = append(networkIDs, network.ID)
	}

	statuses, err := meraki.GetApplianceUplinkStatuses(ctx, config.Meraki, networkIDs)
	if err != nil {
		return err
	}

	locations, err := azure.GetCANamedLocations(ctx, graphClient)
	if err != nil {
		return err
	}

	var errs []error

	for networkID, desired := range buildDesiredIPSet(statuses) {
		networkName, ok := nameByNetworkID[networkID]
		if !ok || networkName == "" {
			errs = append(errs, fmt.Errorf("network %s: no network name known; skipping", networkID))
			continue
		}

		err := azure.KeepNetworksInSync(ctx, graphClient, networkID, networkName, desired, locations)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to synchronize Meraki network %s with Azure: %w", networkID, err))
		}
	}

	return errors.Join(errs...)
}
