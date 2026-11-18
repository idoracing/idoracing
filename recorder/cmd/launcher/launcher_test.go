package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMozaFrameBuilder(t *testing.T) {
	// Frame structure: 7E | length | group | dev_id | cmd_id... | payload... | checksum
	// Where length = len(cmd_id) + len(payload)
	frame := buildMozaFrame(0x3F, 0x17, []byte{0x1A, 0x00}, []byte{0xFF, 0x03})

	if len(frame) != 9 {
		t.Fatalf("expected 9 bytes, got %d: %X", len(frame), frame)
	}
	if frame[0] != 0x7E {
		t.Errorf("frame header = 0x%02X; want 0x7E", frame[0])
	}
	if frame[1] != 4 { // 2 bytes cmd_id + 2 bytes payload
		t.Errorf("length = %d; want 4", frame[1])
	}
	if frame[2] != 0x3F {
		t.Errorf("group = 0x%02X; want 0x3F", frame[2])
	}
	if frame[3] != 0x17 {
		t.Errorf("devID = 0x%02X; want 0x17", frame[3])
	}

	// Verify checksum: (13 + sum(frame[:-1])) % 256
	var sum int = int(MozaMagicChecksum)
	for _, b := range frame[:len(frame)-1] {
		sum += int(b)
	}
	expectedCRC := byte(sum % 256)
	if frame[len(frame)-1] != expectedCRC {
		t.Errorf("checksum = 0x%02X; want 0x%02X", frame[len(frame)-1], expectedCRC)
	}
}

func TestMozaPaletteEncoding(t *testing.T) {
	palette := encodeMozaPalette()
	if len(palette) == 0 {
		t.Fatal("empty palette generated")
	}

	// Two devices (0x17 and 0x15), 6 commands per device
	// Verify that telemetry mode, palette chunks, brightness, and idle effect are included
	if palette[0] != 0x7E {
		t.Errorf("palette byte 0 = 0x%02X; want 0x7E", palette[0])
	}
}

func TestMozaCalcBitmask(t *testing.T) {
	maxRPM := 8000.0

	// Below 75% (6000 RPM) -> 0
	if m := calcMozaBitmask(5000, maxRPM, time.Now()); m != 0 {
		t.Errorf("RPM 5000: mask = 0x%04X; want 0", m)
	}

	// In progressive range (75% - 96% -> 6000 - 7680 RPM)
	mMid := calcMozaBitmask(6850, maxRPM, time.Now())
	if mMid == 0 || mMid == 0x03FF {
		t.Errorf("RPM 6850: expected progressive mask, got 0x%04X", mMid)
	}

	// Above 96% -> Shift flash toggle
	tFlashOn := time.UnixMilli(0)
	if m := calcMozaBitmask(7800, maxRPM, tFlashOn); m != 0x03FF {
		t.Errorf("RPM 7800 (flash on): mask = 0x%04X; want 0x03FF", m)
	}

	tFlashOff := time.UnixMilli(80)
	if m := calcMozaBitmask(7800, maxRPM, tFlashOff); m != 0 {
		t.Errorf("RPM 7800 (flash off): mask = 0x%04X; want 0", m)
	}
}

func TestGameDetection(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectedSim string
	}{
		{
			name:        "Steam launch with reaper and ACC",
			args:        []string{"/usr/bin/reaper", "SteamLaunch", "AppId=805550", "--", "/path/to/proton", "waitforexitandrun", "/path/to/acc.exe"},
			expectedSim: "acc",
		},
		{
			name:        "ACC shipping binary",
			args:        []string{"/path/to/proton", "waitforexitandrun", "AC2-Win64-Shipping.exe"},
			expectedSim: "acc",
		},
		{
			name:        "Le Mans Ultimate game exe",
			args:        []string{"/path/to/proton", "run", "Le Mans Ultimate.exe"},
			expectedSim: "lmu",
		},
		{
			name:        "LMU start protected game",
			args:        []string{"/path/to/proton", "run", "start_protected_game.exe"},
			expectedSim: "lmu",
		},
		{
			name:        "Assetto Corsa EVO",
			args:        []string{"/path/to/proton", "run", "AssettoCorsaEVO.exe"},
			expectedSim: "acevo",
		},
		{
			name:        "Assetto Corsa Rally acr.exe",
			args:        []string{"/path/to/proton", "run", "acr.exe"},
			expectedSim: "acrally",
		},
		{
			name:        "Assetto Corsa Rally full name",
			args:        []string{"/path/to/proton", "run", "AssettoCorsaRally.exe"},
			expectedSim: "acrally",
		},
		{
			name:        "Assetto Corsa 1 acs.exe",
			args:        []string{"/path/to/proton", "run", "acs.exe"},
			expectedSim: "ac",
		},
		{
			name:        "Assetto Corsa 1 AssettoCorsa.exe",
			args:        []string{"/path/to/proton", "run", "AssettoCorsa.exe"},
			expectedSim: "ac",
		},
		{
			name:        "DiRT Rally 2.0 dirtrally2.exe",
			args:        []string{"/path/to/proton", "run", "dirtrally2.exe"},
			expectedSim: "dr2",
		},
		{
			name:        "EA SPORTS WRC WRC.exe",
			args:        []string{"/path/to/proton", "run", "WRC.exe"},
			expectedSim: "dr2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			det := DetectGameFromArgs(tc.args)
			if det.Sim != tc.expectedSim {
				t.Errorf("args %v: detected sim = %s; want %s", tc.args, det.Sim, tc.expectedSim)
			}
		})
	}
}

func TestGameDetectionEnvOverrides(t *testing.T) {
	origSim := os.Getenv("IDORACING_SIM")
	origExe := os.Getenv("IDORACING_GAME_EXE")
	defer func() {
		_ = os.Setenv("IDORACING_SIM", origSim)
		_ = os.Setenv("IDORACING_GAME_EXE", origExe)
	}()

	_ = os.Setenv("IDORACING_SIM", "lmu")
	_ = os.Setenv("IDORACING_GAME_EXE", "custom_game.exe")

	det := DetectGameFromArgs([]string{"/path/to/proton", "run", "acc.exe"})
	if det.Sim != "lmu" {
		t.Errorf("expected sim override 'lmu', got %s", det.Sim)
	}
	if det.GameExe != "custom_game.exe" {
		t.Errorf("expected game exe override 'custom_game.exe', got %s", det.GameExe)
	}
}

func TestDefaultExeForSim(t *testing.T) {
	sims := []string{"acc", "lmu", "acevo", "ac", "acrally", "dr2"}
	for _, sim := range sims {
		exe := DefaultExeForSim(sim)
		if exe == "" {
			t.Errorf("DefaultExeForSim(%s) returned empty string", sim)
		}
	}
}

func TestSetupAntiHangEnvironment(t *testing.T) {
	SetupAntiHangEnvironment()

	expectedEnvs := map[string]string{
		"DXVK_ENABLE_NVAPI":                     "0",
		"PROTON_ENABLE_NVAPI":                   "0",
		"PROTON_DISABLE_NVAPI":                  "1",
		"PROTON_USE_XALIA":                      "0",
		"DISABLE_VK_LAYER_VALVE_steam_overlay_1": "1",
		"SDL_GAMECONTROLLER_IGNORE_DEVICES":     "0x3434/0x0a81",
	}

	for k, v := range expectedEnvs {
		if val := os.Getenv(k); val != v {
			t.Errorf("env %s = %q; want %q", k, val, v)
		}
	}
}

func TestConfigManagerReload(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "spotter_config.json")

	initial := LauncherConfig{
		PepSpotter:      true,
		HUDOverlay:      true,
		MozaShiftLights: true,
		Recorder:        true,
		Enabled:         true,
		HUDEnabled:      true,
		MozaLEDs:        true,
		RecorderEnabled: true,
		Language:        "en",
		Volume:          75,
	}
	data, _ := json.Marshal(initial)
	_ = os.WriteFile(cfgPath, data, 0644)

	cm := NewConfigManager(cfgPath)
	cfg := cm.Get()
	if !cfg.Enabled || cfg.Language != "en" || cfg.Volume != 75 {
		t.Fatalf("unexpected loaded config: %+v", cfg)
	}

	// Update on disk
	updated := LauncherConfig{
		PepSpotter:      false,
		HUDOverlay:      false,
		MozaShiftLights: false,
		Recorder:        false,
		Enabled:         false,
		HUDEnabled:      false,
		MozaLEDs:        false,
		RecorderEnabled: false,
		Language:        "es",
		Volume:          50,
	}
	data2, _ := json.Marshal(updated)
	_ = os.WriteFile(cfgPath, data2, 0644)

	cfg2 := cm.Reload()
	if cfg2.Enabled || cfg2.Language != "es" || cfg2.Volume != 50 || cfg2.HUDEnabled || cfg2.MozaLEDs || cfg2.Recorder {
		t.Fatalf("unexpected reloaded config: %+v", cfg2)
	}
}

func TestConfigManagerNonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "does_not_exist.json")

	cm := NewConfigManager(nonExistent)
	cfg := cm.Get()

	// Should not fail and should use defaults
	if !cfg.PepSpotter || !cfg.HUDOverlay || !cfg.MozaShiftLights || !cfg.Recorder {
		t.Fatalf("expected all defaults to be true, got %+v", cfg)
	}

	// File should NOT have been created on disk
	if _, err := os.Stat(nonExistent); !os.IsNotExist(err) {
		t.Fatalf("expected file to not be created, but it exists: %s", nonExistent)
	}
}

func TestLauncherConfigJSONAliases(t *testing.T) {
	// 1. Modern keys
	modernJSON := `{
		"pep_spotter": false,
		"hud_overlay": false,
		"moza_shift_lights": false,
		"recorder": false,
		"language": "en",
		"volume": 70
	}`
	var cfg1 LauncherConfig
	if err := json.Unmarshal([]byte(modernJSON), &cfg1); err != nil {
		t.Fatalf("failed to unmarshal modern json: %v", err)
	}
	if cfg1.PepSpotter || cfg1.Enabled || cfg1.HUDOverlay || cfg1.HUDEnabled ||
		cfg1.MozaShiftLights || cfg1.MozaLEDs || cfg1.Recorder || cfg1.RecorderEnabled {
		t.Errorf("expected all subsystems disabled in modern json, got %+v", cfg1)
	}
	if cfg1.Language != "en" || cfg1.Volume != 70 {
		t.Errorf("expected lang=en vol=70, got lang=%s vol=%d", cfg1.Language, cfg1.Volume)
	}

	// 2. Legacy keys
	legacyJSON := `{
		"enabled": false,
		"hud_enabled": false,
		"moza_leds": false,
		"recorder_enabled": false
	}`
	var cfg2 LauncherConfig
	if err := json.Unmarshal([]byte(legacyJSON), &cfg2); err != nil {
		t.Fatalf("failed to unmarshal legacy json: %v", err)
	}
	if cfg2.PepSpotter || cfg2.Enabled || cfg2.HUDOverlay || cfg2.HUDEnabled ||
		cfg2.MozaShiftLights || cfg2.MozaLEDs || cfg2.Recorder || cfg2.RecorderEnabled {
		t.Errorf("expected all subsystems disabled in legacy json, got %+v", cfg2)
	}

	// 3. Serialization exports both sets of keys
	outBytes, err := json.Marshal(cfg1)
	if err != nil {
		t.Fatalf("failed to marshal LauncherConfig: %v", err)
	}
	outStr := string(outBytes)
	for _, key := range []string{"pep_spotter", "hud_overlay", "moza_shift_lights", "recorder", "enabled", "hud_enabled", "moza_leds", "recorder_enabled"} {
		if !strings.Contains(outStr, key) {
			t.Errorf("marshaled json missing key %q: %s", key, outStr)
		}
	}
}

func TestFindConfigFileHierarchy(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Explicit path
	custom := filepath.Join(tmpDir, "custom.json")
	if p := FindConfigFile(custom, tmpDir); p != custom {
		t.Errorf("expected custom path %s, got %s", custom, p)
	}

	// 2. IDORACING_CONFIG env
	envFile := filepath.Join(tmpDir, "env_config.json")
	_ = os.WriteFile(envFile, []byte("{}"), 0644)
	origEnv := os.Getenv("IDORACING_CONFIG")
	_ = os.Setenv("IDORACING_CONFIG", envFile)
	defer os.Setenv("IDORACING_CONFIG", origEnv)

	if p := FindConfigFile("", tmpDir); p != envFile {
		t.Errorf("expected env path %s, got %s", envFile, p)
	}
	_ = os.Unsetenv("IDORACING_CONFIG")

	// 3. In repo data directory
	repoDir := filepath.Join(tmpDir, "repo")
	repoData := filepath.Join(repoDir, "data")
	_ = os.MkdirAll(repoData, 0755)
	spotterPath := filepath.Join(repoData, "spotter_config.json")
	_ = os.WriteFile(spotterPath, []byte("{}"), 0644)

	if p := FindConfigFile("", repoDir); filepath.Base(p) != "spotter_config.json" {
		t.Errorf("expected spotter_config.json from repo data, got %s", p)
	}

	// If idoracing.json also exists in repo data, it takes precedence
	idoracingPath := filepath.Join(repoData, "idoracing.json")
	_ = os.WriteFile(idoracingPath, []byte("{}"), 0644)
	if p := FindConfigFile("", repoDir); filepath.Base(p) != "idoracing.json" {
		t.Errorf("expected idoracing.json to take precedence, got %s", p)
	}
}

func TestApplyFlagOverridesToConfig(t *testing.T) {
	// Base configuration with all enabled
	cfg := DefaultConfig()

	fset := flag.NewFlagSet("test", flag.ContinueOnError)
	_ = fset.Bool("hud", true, "")
	_ = fset.Bool("no-hud", false, "")
	_ = fset.Bool("moza", true, "")
	_ = fset.Bool("no-moza", false, "")
	_ = fset.Bool("spotter", true, "")
	_ = fset.Bool("no-spotter", false, "")
	_ = fset.Bool("pep", true, "")
	_ = fset.Bool("no-pep", false, "")
	_ = fset.Bool("recorder", true, "")
	_ = fset.Bool("no-recorder", false, "")

	// Parse disabling flags
	args := []string{"-no-hud", "-no-moza", "-no-pep", "-no-recorder"}
	if err := fset.Parse(args); err != nil {
		t.Fatalf("failed to parse args: %v", err)
	}

	applyFlagOverridesToConfig(&cfg, fset)

	if cfg.HUDOverlay || cfg.HUDEnabled {
		t.Errorf("HUD should be disabled by -no-hud")
	}
	if cfg.MozaShiftLights || cfg.MozaLEDs {
		t.Errorf("Moza should be disabled by -no-moza")
	}
	if cfg.PepSpotter || cfg.Enabled {
		t.Errorf("Spotter should be disabled by -no-pep")
	}
	if cfg.Recorder || cfg.RecorderEnabled {
		t.Errorf("Recorder should be disabled by -no-recorder")
	}

	// Re-enable via explicit flag=true
	fset2 := flag.NewFlagSet("test2", flag.ContinueOnError)
	_ = fset2.Bool("hud", true, "")
	_ = fset2.Bool("moza", true, "")
	_ = fset2.Bool("spotter", true, "")
	_ = fset2.Bool("recorder", true, "")
	_ = fset2.Parse([]string{"-hud=true", "-moza=true", "-spotter=true", "-recorder=true"})

	applyFlagOverridesToConfig(&cfg, fset2)
	if !cfg.HUDOverlay || !cfg.MozaShiftLights || !cfg.PepSpotter || !cfg.Recorder {
		t.Errorf("All subsystems should be re-enabled, got %+v", cfg)
	}
}

func TestBuildRecorderCommand(t *testing.T) {
	ctx := context.Background()

	// 1. Direct proton
	args1 := []string{"/path/to/proton", "waitforexitandrun", "acc.exe"}
	cmd1 := BuildRecorderCommand(ctx, args1, "/path/to/proton", "/path/to/recorder.exe", "acc", "acc.exe", "/path/to/log")
	if cmd1 == nil {
		t.Fatal("expected non-nil cmd1")
	}
	expected1 := []string{"/path/to/proton", "run", "/path/to/recorder.exe", "-exit-with", "acc.exe", "-log", "/path/to/log"}
	if strings.Join(cmd1.Args, " ") != strings.Join(expected1, " ") {
		t.Errorf("cmd1 = %v; want %v", cmd1.Args, expected1)
	}

	// 2. Runtime entrypoint with reaper
	args2 := []string{"/usr/bin/reaper", "SteamLaunch", "--", "_v2-entry-point", "--verb=waitforexitandrun", "--", "/path/to/proton", "waitforexitandrun", "acc.exe"}
	cmd2 := BuildRecorderCommand(ctx, args2, "/path/to/proton", "/path/to/recorder.exe", "acc", "acc.exe", "/path/to/log")
	if cmd2 == nil {
		t.Fatal("expected non-nil cmd2")
	}
	expected2 := []string{"_v2-entry-point", "--verb=run", "--", "/path/to/proton", "run", "/path/to/recorder.exe", "-exit-with", "acc.exe", "-log", "/path/to/log"}
	if strings.Join(cmd2.Args, " ") != strings.Join(expected2, " ") {
		t.Errorf("cmd2 = %v; want %v", cmd2.Args, expected2)
	}

	// 3. Simulator-specific flags (e.g. LMU, ACRally, DR2)
	simTests := []struct {
		sim     string
		gameExe string
		wantSim bool
	}{
		{"lmu", "Le Mans Ultimate.exe,start_protected_game.exe", true},
		{"acrally", "acr.exe,AssettoCorsaRally.exe", true},
		{"acevo", "AssettoCorsaEVO.exe", true},
		{"ac", "acs.exe,AssettoCorsa.exe", true},
		{"dr2", "dirtrally2.exe,WRC.exe", true},
		{"acc", "AC2-Win64-Shipping.exe,acc.exe", false},
	}

	for _, st := range simTests {
		cmd := BuildRecorderCommand(ctx, args1, "/path/to/proton", "/path/to/recorder.exe", st.sim, st.gameExe, "/path/to/log")
		cmdStr := strings.Join(cmd.Args, " ")
		if st.wantSim {
			if !strings.Contains(cmdStr, "-sim "+st.sim) {
				t.Errorf("sim %s: expected '-sim %s' in command %q", st.sim, st.sim, cmdStr)
			}
		} else {
			if strings.Contains(cmdStr, "-sim") {
				t.Errorf("default sim %s should not include '-sim' flag in %q", st.sim, cmdStr)
			}
		}
		if !strings.Contains(cmdStr, "-exit-with "+st.gameExe) {
			t.Errorf("sim %s: expected '-exit-with %s' in %q", st.sim, st.gameExe, cmdStr)
		}
	}
}
