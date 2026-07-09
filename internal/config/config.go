package config

import (
	"errors"
	"log"
	"os"
	"strings"
)

type Config struct {
	Azure  AzureConfig
	Meraki MerakiConfig
}

type AzureConfig struct {
	TenantID     string
	ClientID     string
	ClientSecret string
}

type MerakiConfig struct {
	ClientSecret       string
	OrganizationID     string
	AllowedNetworkTags []string
}

func LoadFromEnv() (Config, error) {
	config := Config{
		Azure: AzureConfig{
			TenantID:     os.Getenv("AZURE_TENANT_ID"),
			ClientID:     os.Getenv("AZURE_CLIENT_ID"),
			ClientSecret: os.Getenv("AZURE_CLIENT_SECRET"),
		},
		Meraki: MerakiConfig{
			ClientSecret:   os.Getenv("MERAKI_CLIENT_SECRET"),
			OrganizationID: os.Getenv("MERAKI_ORGANIZATION_ID"),
		},
	}

	allowedNetworkTags := os.Getenv("MERAKI_ALLOWED_NETWORK_TAGS")

	for networkTag := range strings.SplitSeq(allowedNetworkTags, ",") {
		if networkTag = strings.TrimSpace(networkTag); networkTag != "" {
			config.Meraki.AllowedNetworkTags = append(config.Meraki.AllowedNetworkTags, networkTag)
		}
	}

	required := []struct{ name, value string }{
		{"AZURE_TENANT_ID", config.Azure.TenantID},
		{"AZURE_CLIENT_ID", config.Azure.ClientID},
		{"AZURE_CLIENT_SECRET", config.Azure.ClientSecret},
		{"MERAKI_CLIENT_SECRET", config.Meraki.ClientSecret},
		{"MERAKI_ORGANIZATION_ID", config.Meraki.OrganizationID},
		{"MERAKI_ALLOWED_NETWORK_TAGS", strings.Join(config.Meraki.AllowedNetworkTags, ",")},
	}

	missing := false

	for _, v := range required {
		if v.value == "" {
			log.Printf("missing required environment variable: %s", v.name)
			missing = true
		}
	}

	if missing {
		return Config{}, errors.New("required environment variables are missing; please provide them")
	}

	return config, nil
}
