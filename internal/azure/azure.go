package azure

import (
	"context"
	"fmt"
	"log"
	"maps"
	"net/netip"
	"slices"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	msgraphsdkgo "github.com/microsoftgraph/msgraph-sdk-go"
	msgraphsdkgocore "github.com/microsoftgraph/msgraph-sdk-go-core"
	"github.com/microsoftgraph/msgraph-sdk-go/models"
	"github.com/purpose-robot/meraki-entra-sync/internal/config"
)

type CIDRSet map[string]struct{}

func (c CIDRSet) Sorted() []string {
	return slices.Sorted(maps.Keys(c))
}

type cidrRangeable interface {
	models.IpRangeable
	SetCidrAddress(*string)
}

func newCIDRRange(cidr string) models.IpRangeable {
	var ipRange cidrRangeable

	switch {
	case strings.Contains(cidr, ":"):
		ipRange = models.NewIPv6CidrRange()
	default:
		ipRange = models.NewIPv4CidrRange()
	}

	ipRange.SetCidrAddress(&cidr)
	return ipRange
}

func (c CIDRSet) toIPRanges() []models.IpRangeable {
	ipRanges := make([]models.IpRangeable, 0, len(c))

	for _, cidr := range c.Sorted() {
		ipRanges = append(ipRanges, newCIDRRange(cidr))
	}

	return ipRanges
}

func KeepNetworksInSync(ctx context.Context, client *msgraphsdkgo.GraphServiceClient, networkID, networkName string, desired CIDRSet, locations map[string]NamedLocation) error {
	if len(desired) == 0 {
		log.Printf("network %s (%q): No public IPs reported; skip", networkID, networkName)
		return nil
	}

	existing, ok := locations[networkName]
	if !ok {
		locationID, err := createCANamedLocations(ctx, client, networkName, desired)
		if err != nil {
			return err
		}

		locations[networkName] = NamedLocation{ID: locationID, IPRanges: desired}

		log.Printf("network %s (%q): Named location %s with IP set %v created", networkID, networkName, locationID, desired.Sorted())
		return nil
	}

	if maps.Equal(existing.IPRanges, desired) {
		return nil
	}

	log.Printf("network %s (%q): Public IPs changed from %v to %v", networkID, networkName, existing.IPRanges.Sorted(), desired.Sorted())

	if err := updateCANamedLocations(ctx, client, existing.ID, desired); err != nil {
		return err
	}

	existing.IPRanges = desired
	locations[networkName] = existing

	log.Printf("network %s (%q): Named location %s successfully updated in Azure", networkID, networkName, existing.ID)
	return nil
}

func NewGraphClient(config config.AzureConfig) (*msgraphsdkgo.GraphServiceClient, error) {
	credential, err := azidentity.NewClientSecretCredential(
		config.TenantID, config.ClientID, config.ClientSecret, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create new Azure credential for client with ID %s: %w", config.ClientID, err)
	}

	scopes := []string{"https://graph.microsoft.com/.default"}

	graphClient, err := msgraphsdkgo.NewGraphServiceClientWithCredentials(credential, scopes)
	if err != nil {
		return nil, fmt.Errorf("failed to create new Azure graph client for client with ID %s: %w", config.ClientID, err)
	}

	return graphClient, nil
}

type NamedLocation struct {
	ID       string
	IPRanges CIDRSet
}

func normalizeCIDR(cidr string) string {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return cidr
	}

	return prefix.Masked().String()
}

func GetCANamedLocations(ctx context.Context, client *msgraphsdkgo.GraphServiceClient) (map[string]NamedLocation, error) {
	response, err := client.Identity().ConditionalAccess().NamedLocations().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve named locations: %w", err)
	}

	iterator, err := msgraphsdkgocore.NewPageIterator[models.NamedLocationable](response, client.GetAdapter(), models.CreateNamedLocationCollectionResponseFromDiscriminatorValue)
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator from named location: %w", err)
	}

	locations := make(map[string]NamedLocation)

	err = iterator.Iterate(ctx, func(pageItem models.NamedLocationable) bool {
		namedLocation, ok := pageItem.(models.IpNamedLocationable)
		if !ok || namedLocation.GetId() == nil || namedLocation.GetDisplayName() == nil {
			return true
		}

		location := NamedLocation{
			ID:       *namedLocation.GetId(),
			IPRanges: make(CIDRSet),
		}

		for _, ipRange := range namedLocation.GetIpRanges() {
			var cidr *string

			switch v := ipRange.(type) {
			case *models.IPv4CidrRange:
				cidr = v.GetCidrAddress()

			case *models.IPv6CidrRange:
				cidr = v.GetCidrAddress()
			}

			if cidr != nil {
				location.IPRanges[normalizeCIDR(*cidr)] = struct{}{}
			}
		}

		name := *namedLocation.GetDisplayName()

		if _, exists := locations[name]; exists {
			log.Printf("multiple named locations share the display name %q; only syncing last one returned", name)
		}

		locations[name] = location
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate over existing named locations: %w", err)
	}

	return locations, nil
}

func updateCANamedLocations(ctx context.Context, client *msgraphsdkgo.GraphServiceClient, locationID string, cidrs CIDRSet) error {
	body := models.NewIpNamedLocation()
	body.SetIpRanges(cidrs.toIPRanges())

	_, err := client.Identity().ConditionalAccess().NamedLocations().ByNamedLocationId(locationID).Patch(ctx, body, nil)
	if err != nil {
		return fmt.Errorf("failed to update named location %q: %w", locationID, err)
	}

	return nil
}

func createCANamedLocations(ctx context.Context, client *msgraphsdkgo.GraphServiceClient, name string, cidrs CIDRSet) (string, error) {
	body := models.NewIpNamedLocation()

	body.SetDisplayName(&name)
	body.SetIsTrusted(new(true))
	body.SetIpRanges(cidrs.toIPRanges())

	created, err := client.Identity().ConditionalAccess().NamedLocations().Post(ctx, body, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create new named location %q: %w", name, err)
	}

	if created.GetId() == nil {
		return "", fmt.Errorf("named location %s successfully created but received no identifier", name)
	}

	return *created.GetId(), nil
}
