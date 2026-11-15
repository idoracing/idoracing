package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type DetectedGame struct {
	Sim        string
	GameExe    string
	ProtonPath string
	GameArgs   []string
}

func DefaultExeForSim(sim string) string {
	switch sim {
	case "acrally":
		return "acr.exe,AssettoCorsaRally.exe,ACR-Win64-Shipping.exe,AssettoCorsaRally-Win64-Shipping.exe,acrally.exe"
	case "acc":
		return "AC2-Win64-Shipping.exe,acc.exe"
	case "lmu":
		return "Le Mans Ultimate.exe,start_protected_game.exe"
	case "acevo":
		return "AssettoCorsaEVO.exe"
	case "ac":
		return "acs.exe,AssettoCorsa.exe"
	case "dr2":
		return "dirtrally2.exe,WRC.exe"
	default:
		return "AC2-Win64-Shipping.exe,acc.exe"
	}
}

// DetectGameFromArgs inspects Steam's launch args (with potential reaper / Proton wrappers)
// and extracts the simulator type, target game executable, and Proton path.
func DetectGameFromArgs(args []string) DetectedGame {
	var g DetectedGame
	g.Sim = "acc"
	g.GameExe = "AC2-Win64-Shipping.exe,acc.exe"

	cmdStr := strings.ToLower(strings.Join(args, " "))

	if strings.Contains(cmdStr, "acr.exe") || strings.Contains(cmdStr, "assettocorsarally") || strings.Contains(cmdStr, "acrally") {
		g.Sim = "acrally"
		g.GameExe = "acr.exe,AssettoCorsaRally.exe,ACR-Win64-Shipping.exe,AssettoCorsaRally-Win64-Shipping.exe,acrally.exe"
	} else if strings.Contains(cmdStr, "ac2-win64-shipping.exe") || strings.Contains(cmdStr, "acc.exe") || strings.Contains(cmdStr, "assetto corsa competizione") {
		g.Sim = "acc"
		g.GameExe = "AC2-Win64-Shipping.exe,acc.exe"
	} else if strings.Contains(cmdStr, "le mans ultimate") || strings.Contains(cmdStr, "start_protected_game.exe") {
		g.Sim = "lmu"
		g.GameExe = "Le Mans Ultimate.exe,start_protected_game.exe"
	} else if strings.Contains(cmdStr, "assettocorsaevo") || strings.Contains(cmdStr, "assetto corsa evo") {
		g.Sim = "acevo"
		g.GameExe = "AssettoCorsaEVO.exe"
	} else if strings.Contains(cmdStr, "acs.exe") || strings.Contains(cmdStr, "assettocorsa.exe") || strings.Contains(cmdStr, "assetto corsa") {
		g.Sim = "ac"
		g.GameExe = "acs.exe,AssettoCorsa.exe"
	} else if strings.Contains(cmdStr, "dirtrally2.exe") || strings.Contains(cmdStr, "wrc.exe") || strings.Contains(cmdStr, "dirt rally 2.0") || strings.Contains(cmdStr, "ea sports wrc") {
		g.Sim = "dr2"
		g.GameExe = "dirtrally2.exe,WRC.exe"
	}

	if v := os.Getenv("IDORACING_SIM"); v != "" && v != "auto" {
		g.Sim = v
	}
	if v := os.Getenv("IDORACING_GAME_EXE"); v != "" {
		g.GameExe = v
	}

	for _, arg := range args {
		if strings.HasSuffix(arg, "/proton") || filepath.Base(arg) == "proton" {
			g.ProtonPath = arg
			break
		}
	}
	g.GameArgs = args
	return g
}

// SetupAntiHangEnvironment sets essential Linux/Proton environment variables
// that prevent deadlocks on Nvidia Linux and Keychron keyboard hangs.
func SetupAntiHangEnvironment() {
	_ = os.Setenv("DXVK_ENABLE_NVAPI", "0")
	_ = os.Setenv("PROTON_ENABLE_NVAPI", "0")
	_ = os.Setenv("PROTON_DISABLE_NVAPI", "1")
	_ = os.Setenv("PROTON_USE_XALIA", "0")
	_ = os.Setenv("DISABLE_VK_LAYER_VALVE_steam_overlay_1", "1")

	if os.Getenv("SDL_GAMECONTROLLER_IGNORE_DEVICES") == "" {
		_ = os.Setenv("SDL_GAMECONTROLLER_IGNORE_DEVICES", "0x3434/0x0a81")
	}

	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") != "" {
		_ = os.Setenv("DISPLAY", ":0")
	}
}

// BuildRecorderCommand constructs the Proton command line to execute idoracing-recorder.exe in Wine.
func BuildRecorderCommand(ctx context.Context, args []string, protonPath, recorderExe, sim, gameExe, logPath string) *exec.Cmd {
	// Filter out reaper prefix if present
	recArgs := make([]string, 0, len(args)+8)
	hasReaper := false
	for _, a := range args {
		if strings.HasSuffix(a, "reaper") {
			hasReaper = true
			break
		}
	}

	inReaper := false
	pastReaper := !hasReaper

	for _, a := range args {
		if hasReaper && !pastReaper {
			if strings.HasSuffix(a, "reaper") {
				inReaper = true
				continue
			}
			if inReaper && a == "--" {
				pastReaper = true
			}
			continue
		}

		if a == "--verb=waitforexitandrun" {
			recArgs = append(recArgs, "--verb=run")
		} else {
			recArgs = append(recArgs, a)
		}

		if strings.HasSuffix(a, "/proton") || filepath.Base(a) == "proton" {
			recArgs = append(recArgs, "run", recorderExe)
			explicitSim := os.Getenv("IDORACING_EXPLICIT_SIM") == "1"
			if sim != "" && sim != "auto" && (explicitSim || sim != "acc") {
				recArgs = append(recArgs, "-sim", sim)
			}
			recArgs = append(recArgs, "-exit-with", gameExe, "-log", logPath)
			break
		}
	}

	if len(recArgs) == 0 {
		return nil
	}
	return exec.CommandContext(ctx, recArgs[0], recArgs[1:]...)
}

// RunHUDOverlay starts the compiled idoracing-hud overlay binary.
func RunHUDOverlay(ctx context.Context, hudBin, configFile string, logger *Logger) {
	if fi, err := os.Stat(hudBin); err != nil || fi.IsDir() {
		return
	}

	logger.Info("HUD", "Starting Pedal & Trail Braking HUD overlay (%s)", hudBin)
	cmd := exec.CommandContext(ctx, hudBin, "-config", configFile)
	cmd.Stdout = logger.SubsysWriter("HUD")
	cmd.Stderr = logger.SubsysWriter("HUD")

	if os.Getenv("IDORACING_RECORDER_DELAY") != "0" {
		time.Sleep(3 * time.Second) // Small delay so game window initializes first
	}
	if err := cmd.Start(); err != nil {
		logger.Warn("HUD", "Could not start HUD: %v", err)
		return
	}

	go func() {
		_ = cmd.Wait()
		logger.Info("HUD", "Overlay terminated")
	}()
}
