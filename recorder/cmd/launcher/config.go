package main

import (
	"encoding/json"
	"os"
	"sync"
)

type SpotterConfig struct {
	Enabled    bool   `json:"enabled"`
	Language   string `json:"language"`
	Volume     int    `json:"volume"`
	HUDEnabled bool   `json:"hud_enabled"`
	MozaLEDs   bool   `json:"moza_leds"`
}

type ConfigManager struct {
	path string
	mu   sync.RWMutex
	cfg  SpotterConfig
}

func DefaultConfig() SpotterConfig {
	return SpotterConfig{
		Enabled:    true,
		Language:   "es",
		Volume:     100,
		HUDEnabled: true,
		MozaLEDs:   true,
	}
}

func NewConfigManager(path string) *ConfigManager {
	cm := &ConfigManager{
		path: path,
		cfg:  DefaultConfig(),
	}
	cm.Reload()
	return cm
}

func (cm *ConfigManager) Reload() SpotterConfig {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.path)
	if err != nil {
		return cm.cfg
	}

	var parsed SpotterConfig
	// Populate with current defaults first
	parsed = cm.cfg
	if err := json.Unmarshal(data, &parsed); err == nil {
		if parsed.Volume <= 0 {
			parsed.Volume = 100
		}
		if parsed.Language != "en" && parsed.Language != "es" {
			parsed.Language = "es"
		}
		cm.cfg = parsed
	}
	return cm.cfg
}

func (cm *ConfigManager) Get() SpotterConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.cfg
}
