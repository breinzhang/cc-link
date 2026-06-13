package statusline

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	ccLinkConfigDir  = ".cc-link"
	ccLinkConfigFile = "cc-link.json"
)

type statuslineConfig struct {
	Lines int                  `json:"lines,omitempty"`
	Cost  statuslineCostConfig `json:"cost,omitempty"`
}

type statuslineCostConfig struct {
	Enabled  *bool                     `json:"enabled,omitempty"`
	Currency string                    `json:"currency,omitempty"`
	Prices   []statuslineProviderPrice `json:"prices,omitempty"`
}

type statuslineProviderPrice struct {
	Provider string                 `json:"provider"`
	Models   []statuslineModelPrice `json:"models"`
}

type statuslineModelPrice struct {
	Provider   string  `json:"provider,omitempty"`
	Match      string  `json:"match"`
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheWrite float64 `json:"cacheWrite,omitempty"`
	CacheRead  float64 `json:"cacheRead,omitempty"`
}

type ccLinkConfig struct {
	Statusline statuslineConfig `json:"statusline,omitempty"`
}

func loadStatuslineConfig(projectRoot string) (statuslineConfig, error) {
	cfg := statuslineConfig{Lines: 2}
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, err
	}
	globalPath := filepath.Join(home, ccLinkConfigDir, ccLinkConfigFile)
	if c, ok, err := readStatuslineConfigIfExists(globalPath); err != nil {
		return cfg, err
	} else if ok {
		cfg = mergeStatuslineConfig(cfg, c)
	}

	if projectRoot != "" {
		projectPath := filepath.Join(projectRoot, ccLinkConfigDir, ccLinkConfigFile)
		if c, ok, err := readStatuslineConfigIfExists(projectPath); err != nil {
			return cfg, err
		} else if ok {
			cfg = mergeStatuslineConfig(cfg, c)
		}
	}
	return cfg, nil
}

func readStatuslineConfigIfExists(path string) (statuslineConfig, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return statuslineConfig{}, false, nil
		}
		return statuslineConfig{}, false, err
	}
	var cfg ccLinkConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return statuslineConfig{}, true, err
	}
	return cfg.Statusline, true, nil
}

func mergeStatuslineConfig(base, override statuslineConfig) statuslineConfig {
	if override.Lines > 0 {
		base.Lines = override.Lines
	}
	if override.Cost.Enabled != nil {
		base.Cost.Enabled = override.Cost.Enabled
	}
	if override.Cost.Currency != "" {
		base.Cost.Currency = override.Cost.Currency
	}
	if len(override.Cost.Prices) > 0 {
		base.Cost.Prices = append(append([]statuslineProviderPrice{}, override.Cost.Prices...), base.Cost.Prices...)
	}
	return base
}

func (cfg statuslineCostConfig) isEnabled() bool {
	return cfg.Enabled == nil || *cfg.Enabled
}

func (cfg statuslineCostConfig) modelPrices() []statuslineModelPrice {
	var prices []statuslineModelPrice
	for _, provider := range cfg.Prices {
		for _, model := range provider.Models {
			if model.Provider == "" {
				model.Provider = provider.Provider
			}
			prices = append(prices, model)
		}
	}
	return prices
}
