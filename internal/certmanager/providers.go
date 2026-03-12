package certmanager

import (
	"encoding/json"
	"fmt"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/route53"

	"github.com/ngate/internal/models"
)

// buildDNSProvider creates a lego DNS challenge provider from config.
// Uses NewDNSProviderConfig() to avoid process-wide env var mutation.
func buildDNSProvider(providerType models.ProviderType, configJSON string) (challenge.Provider, error) {
	switch providerType {
	case models.ProviderLetsEncryptDNSR53:
		return buildRoute53(configJSON)
	case models.ProviderLetsEncryptDNSCF:
		return buildCloudflare(configJSON)
	default:
		return nil, fmt.Errorf("unsupported DNS provider: %s", providerType)
	}
}

func buildRoute53(configJSON string) (challenge.Provider, error) {
	var cfg struct {
		AccessKeyID    string `json:"access_key_id"`
		SecretAccessKey string `json:"secret_access_key"`
		Region         string `json:"region"`
		HostedZoneID   string `json:"hosted_zone_id"`
	}
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return nil, fmt.Errorf("parse route53 config: %w", err)
	}

	r53cfg := route53.NewDefaultConfig()
	r53cfg.AccessKeyID = cfg.AccessKeyID
	r53cfg.SecretAccessKey = cfg.SecretAccessKey
	r53cfg.Region = cfg.Region
	r53cfg.HostedZoneID = cfg.HostedZoneID

	return route53.NewDNSProviderConfig(r53cfg)
}

func buildCloudflare(configJSON string) (challenge.Provider, error) {
	var cfg struct {
		APIToken string `json:"api_token"`
		APIEmail string `json:"api_email"`
		APIKey   string `json:"api_key"`
	}
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return nil, fmt.Errorf("parse cloudflare config: %w", err)
	}

	cfcfg := cloudflare.NewDefaultConfig()
	if cfg.APIToken != "" {
		cfcfg.AuthToken = cfg.APIToken
	} else if cfg.APIEmail != "" && cfg.APIKey != "" {
		cfcfg.AuthEmail = cfg.APIEmail
		cfcfg.AuthKey = cfg.APIKey
	} else {
		return nil, fmt.Errorf("cloudflare requires api_token or (api_email + api_key)")
	}

	return cloudflare.NewDNSProviderConfig(cfcfg)
}
