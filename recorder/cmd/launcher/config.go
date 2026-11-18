package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// LauncherConfig defines the supervisor subsystem toggles and spotter configuration.
type LauncherConfig struct {
	// Subsystem toggles
	PepSpotter      bool `json:"pep_spotter"`
	HUDOverlay      bool `json:"hud_overlay"`
	MozaShiftLights bool `json:"moza_shift_lights"`
	Recorder        bool `json:"recorder"`

	// Legacy aliases for backward compatibility
	Enabled         bool `json:"enabled"`
	HUDEnabled      bool `json:"hud_enabled"`
	MozaLEDs        bool `json:"moza_leds"`
	RecorderEnabled bool `json:"recorder_enabled"`

	// Spotter & radio preferences
	Language     string `json:"language"`
	Volume       int    `json:"volume"`
	ToggleKey    string `json:"toggle_key,omitempty"`
	ToggleVkCode int    `json:"toggle_vk_code,omitempty"`
}

// SpotterConfig is an alias for LauncherConfig to maintain 100% backward compatibility.
type SpotterConfig = LauncherConfig

func DefaultConfig() LauncherConfig {
	return LauncherConfig{
		PepSpotter:      true,
		HUDOverlay:      true,
		MozaShiftLights: true,
		Recorder:        true,
		Enabled:         true,
		HUDEnabled:      true,
		MozaLEDs:        true,
		RecorderEnabled: true,
		Language:        "es",
		Volume:          85,
		ToggleKey:       "F9",
		ToggleVkCode:    120,
	}
}

func (c *LauncherConfig) UnmarshalJSON(data []byte) error {
	*c = DefaultConfig()

	type rawConfig struct {
		PepSpotterRaw      *bool  `json:"pep_spotter"`
		HUDOverlayRaw      *bool  `json:"hud_overlay"`
		MozaShiftLightsRaw *bool  `json:"moza_shift_lights"`
		RecorderRaw        *bool  `json:"recorder"`
		EnabledRaw         *bool  `json:"enabled"`
		HUDEnabledRaw      *bool  `json:"hud_enabled"`
		MozaLEDsRaw        *bool  `json:"moza_leds"`
		RecorderEnabledRaw *bool  `json:"recorder_enabled"`
		Language           string `json:"language"`
		Volume             *int   `json:"volume"`
		ToggleKey          string `json:"toggle_key"`
		ToggleVkCode       int    `json:"toggle_vk_code"`
	}

	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// 1. Pep Spotter / Enabled
	if raw.PepSpotterRaw != nil {
		c.PepSpotter = *raw.PepSpotterRaw
	} else if raw.EnabledRaw != nil {
		c.PepSpotter = *raw.EnabledRaw
	}
	c.Enabled = c.PepSpotter

	// 2. HUD Overlay / HUD Enabled
	if raw.HUDOverlayRaw != nil {
		c.HUDOverlay = *raw.HUDOverlayRaw
	} else if raw.HUDEnabledRaw != nil {
		c.HUDOverlay = *raw.HUDEnabledRaw
	}
	c.HUDEnabled = c.HUDOverlay

	// 3. Moza Shift Lights / Moza LEDs
	if raw.MozaShiftLightsRaw != nil {
		c.MozaShiftLights = *raw.MozaShiftLightsRaw
	} else if raw.MozaLEDsRaw != nil {
		c.MozaShiftLights = *raw.MozaLEDsRaw
	}
	c.MozaLEDs = c.MozaShiftLights

	// 4. Recorder / Recorder Enabled
	if raw.RecorderRaw != nil {
		c.Recorder = *raw.RecorderRaw
	} else if raw.RecorderEnabledRaw != nil {
		c.Recorder = *raw.RecorderEnabledRaw
	}
	c.RecorderEnabled = c.Recorder

	// 5. Radio & spotter preferences
	if raw.Language != "" {
		if raw.Language == "en" || raw.Language == "es" {
			c.Language = raw.Language
		}
	}
	if raw.Volume != nil {
		vol := *raw.Volume
		if vol < 0 {
			vol = 0
		} else if vol > 100 {
			vol = 100
		}
		c.Volume = vol
	}
	if raw.ToggleKey != "" {
		c.ToggleKey = raw.ToggleKey
	}
	if raw.ToggleVkCode > 0 {
		c.ToggleVkCode = raw.ToggleVkCode
	}

	return nil
}

func (c LauncherConfig) MarshalJSON() ([]byte, error) {
	type outputConfig struct {
		PepSpotter      bool   `json:"pep_spotter"`
		HUDOverlay      bool   `json:"hud_overlay"`
		MozaShiftLights bool   `json:"moza_shift_lights"`
		Recorder        bool   `json:"recorder"`
		Enabled         bool   `json:"enabled"`
		HUDEnabled      bool   `json:"hud_enabled"`
		MozaLEDs        bool   `json:"moza_leds"`
		RecorderEnabled bool   `json:"recorder_enabled"`
		Language        string `json:"language"`
		Volume          int    `json:"volume"`
		ToggleKey       string `json:"toggle_key,omitempty"`
		ToggleVkCode    int    `json:"toggle_vk_code,omitempty"`
	}

	out := outputConfig{
		PepSpotter:      c.PepSpotter,
		HUDOverlay:      c.HUDOverlay,
		MozaShiftLights: c.MozaShiftLights,
		Recorder:        c.Recorder,
		Enabled:         c.PepSpotter,
		HUDEnabled:      c.HUDOverlay,
		MozaLEDs:        c.MozaShiftLights,
		RecorderEnabled: c.Recorder,
		Language:        c.Language,
		Volume:          c.Volume,
		ToggleKey:       c.ToggleKey,
		ToggleVkCode:    c.ToggleVkCode,
	}
	return json.MarshalIndent(out, "", "  ")
}

type ConfigManager struct {
	path string
	mu   sync.RWMutex
	cfg  LauncherConfig
}

// FindConfigFile locates the configuration file in standard hierarchical paths.
func FindConfigFile(customPath, repoRoot string) string {
	if customPath != "" {
		return customPath
	}

	if envPath := os.Getenv("IDORACING_CONFIG"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	fileNames := []string{"idoracing.json", "config.json", "spotter_config.json"}
	var dirs []string

	// 1. Current working directory
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, cwd, filepath.Join(cwd, "data"))
	}

	// 2. Executable directory & relative parents
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		dirs = append(dirs, exeDir, filepath.Join(exeDir, "data"))
		dirs = append(dirs, filepath.Join(exeDir, ".."), filepath.Join(exeDir, "..", "data"))
		dirs = append(dirs, filepath.Join(exeDir, "..", ".."), filepath.Join(exeDir, "..", "..", "data"))
	}

	// 3. Repository root
	if repoRoot != "" {
		dirs = append(dirs, filepath.Join(repoRoot, "data"), repoRoot)
	}

	// 4. Standard user config directory
	if userConfigDir, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(userConfigDir, "idoracing"), filepath.Join(userConfigDir, "IdoRacing"))
	}

	for _, dir := range dirs {
		for _, name := range fileNames {
			cand := filepath.Join(dir, name)
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				if abs, err := filepath.Abs(cand); err == nil {
					return abs
				}
				return cand
			}
		}
	}

	// Default fallback path for auto-creation
	if repoRoot != "" {
		dataDir := filepath.Join(repoRoot, "data")
		if fi, err := os.Stat(dataDir); err == nil && fi.IsDir() {
			return filepath.Join(dataDir, "idoracing.json")
		}
	}

	if userConfigDir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(userConfigDir, "idoracing", "config.json")
	}

	return filepath.Join("data", "idoracing.json")
}

func NewConfigManager(path string) *ConfigManager {
	cm := &ConfigManager{
		path: path,
		cfg:  DefaultConfig(),
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = cm.Save(cm.cfg)
	} else {
		cm.Reload()
	}

	return cm
}

func (cm *ConfigManager) Reload() LauncherConfig {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := os.ReadFile(cm.path)
	if err != nil {
		return cm.cfg
	}

	var parsed LauncherConfig
	if err := json.Unmarshal(data, &parsed); err == nil {
		cm.cfg = parsed
	}
	return cm.cfg
}

func (cm *ConfigManager) Get() LauncherConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.cfg
}

func (cm *ConfigManager) Set(cfg LauncherConfig) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.cfg = cfg
}

func (cm *ConfigManager) Path() string {
	return cm.path
}

func (cm *ConfigManager) Save(cfg LauncherConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(cm.path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	if err := os.WriteFile(cm.path, data, 0644); err != nil {
		return err
	}

	cm.cfg = cfg
	return nil
}
