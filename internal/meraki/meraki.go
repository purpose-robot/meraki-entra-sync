package meraki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"

	"github.com/purpose-robot/meraki-entra-sync/internal/config"
)

const baseURL = "https://api.meraki.com/api/v1"

type Networks []struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ApplianceUplinks []struct {
	Status   string     `json:"status"`
	PublicIP netip.Addr `json:"publicIp"`
}

type ApplianceUplinkStatuses []struct {
	Uplinks   ApplianceUplinks `json:"uplinks"`
	NetworkID string           `json:"networkId"`
}

func get(ctx context.Context, apiToken, endpointURL string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+endpointURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request with context: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request to Meraki endpoint: %s; %w", endpointURL, err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("unexpected HTTP error code %d from %s: %s", res.StatusCode, endpointURL, body)
	}

	if err := json.NewDecoder(res.Body).Decode(v); err != nil {
		return fmt.Errorf("failed to decode received response body from Meraki into Go structs: %w", err)
	}

	return nil
}

func GetNetworks(ctx context.Context, config config.MerakiConfig) (Networks, error) {
	q := url.Values{}

	for _, tag := range config.AllowedNetworkTags {
		q.Add("tags[]", tag)
	}

	response := Networks{}
	endpoint := fmt.Sprintf("/organizations/%s/networks?%s", config.OrganizationID, q.Encode())

	if err := get(ctx, config.ClientSecret, endpoint, &response); err != nil {
		return nil, err
	}

	return response, nil
}

func GetApplianceUplinkStatuses(ctx context.Context, config config.MerakiConfig, networkIDs []string) (ApplianceUplinkStatuses, error) {
	q := url.Values{}

	for _, networkID := range networkIDs {
		q.Add("networkIds[]", networkID)
	}

	response := ApplianceUplinkStatuses{}
	endpoint := fmt.Sprintf("/organizations/%s/appliance/uplink/statuses?%s", config.OrganizationID, q.Encode())

	if err := get(ctx, config.ClientSecret, endpoint, &response); err != nil {
		return nil, err
	}

	return response, nil
}
