package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const Version = "0.3.0"

func findRepoRoot() string {
	// 1. Try executable directory
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		// e.g. /repo/recorder/bin -> /repo
		if filepath.Base(dir) == "bin" && filepath.Base(filepath.Dir(dir)) == "recorder" {
			return filepath.Dir(filepath.Dir(dir))
		}
		// e.g. /repo/recorder -> /repo
		if filepath.Base(dir) == "recorder" {
			return filepath.Dir(dir)
		}
	}
	// 2. Try current working directory
	if cwd, err := os.Getwd(); err == nil {
		if fi, err := os.Stat(filepath.Join(cwd, "recorder")); err == nil && fi.IsDir() {
			return cwd
		}
	}
	// 3. Fallback to default
	return "/home/miquel/git/acc-coach"
}

func defaultLogPath(repoRoot string) string {
	if v := os.Getenv("IDORACING_RECORDER_LOG"); v != "" {
		return v
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home := os.Getenv("HOME")
		if home != "" {
			stateHome = filepath.Join(home, ".local", "state")
		}
	}
	if stateHome != "" {
		if err := os.MkdirAll(stateHome, 0755); err == nil {
			testFile := filepath.Join(stateHome, "idoracing-recorder.log")
			if f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
				_ = f.Close()
				return testFile
			}
		}
	}
	dataDir := filepath.Join(repoRoot, "data")
	if err := os.MkdirAll(dataDir, 0755); err == nil {
		return filepath.Join(dataDir, "idoracing-recorder.log")
	}
	return "/tmp/idoracing-recorder.log"
}

func main() {
	flagVersion := flag.Bool("version", false, "Print IdoRacing Launcher version and exit")
	flagStatus := flag.Bool("status", false, "Print hardware and environment diagnostics and exit")
	flagMozaTest := flag.Bool("moza-test", false, "Run instant Moza rev LEDs test pattern and exit")
	flagMozaSim := flag.Bool("moza-simulate", false, "Run Moza dynamic gear pull simulation on wheel and exit")
	flagPort := flag.String("port", "", "Moza CDC-ACM serial port path (default: auto-detect)")
	flagConfig := flag.String("config", "", "Path to spotter_config.json")
	flagLog := flag.String("log", "", "Path to idoracing log file")
	flagSim := flag.String("sim", "auto", "Force simulator override (acc, lmu, acevo, dr2, acrally, ac, auto)")

	flag.Parse()

	if *flagVersion {
		fmt.Printf("IdoRacing Unified Launcher v%s (Linux amd64)\n", Version)
		os.Exit(0)
	}

	repoRoot := findRepoRoot()
	logPath := *flagLog
	if logPath == "" {
		logPath = defaultLogPath(repoRoot)
	}

	logger, err := NewLogger(logPath, LevelInfo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "idoracing-launcher: cannot initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	if *flagStatus {
		logger.Info("LAUNCHER", "IdoRacing Launcher v%s Status Report", Version)
		logger.Info("LAUNCHER", "Repo root: %s", repoRoot)
		logger.Info("LAUNCHER", "Log path:  %s", logPath)
		mozaPort := *flagPort
		if mozaPort == "" {
			mozaPort = findMozaSerialPort()
		}
		if mozaPort != "" {
			logger.Info("LAUNCHER", "Moza hardware detected at: %s", mozaPort)
		} else {
			logger.Info("LAUNCHER", "No Moza CDC-ACM port detected")
		}
		os.Exit(0)
	}

	if *flagMozaTest {
		if err := TestMozaLEDs(*flagPort, logger); err != nil {
			logger.Error("MOZA", "Test failed: %v", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if *flagMozaSim {
		if err := SimulateMozaSweep(*flagPort, 8*time.Second, logger); err != nil {
			logger.Error("MOZA", "Simulation failed: %v", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Game Launch Supervisor Mode
	args := flag.Args()
	if len(args) == 0 {
		_, _ = os.Stdout.WriteString("Usage: idoracing-launcher [options] %command%\n")
		fmt.Println("       idoracing-launcher -status")
		fmt.Println("       idoracing-launcher -moza-test")
		fmt.Println("       idoracing-launcher -moza-simulate")
		os.Exit(1)
	}

	configPath := *flagConfig
	if configPath == "" {
		configPath = filepath.Join(repoRoot, "data", "spotter_config.json")
	}
	cfgMgr := NewConfigManager(configPath)
	cfg := cfgMgr.Get()

	voicesDir := os.Getenv("IDORACING_VOICES_DIR")
	if voicesDir == "" {
		voicesDir = filepath.Join(repoRoot, "recorder", "voices")
	}

	logger.Info("LAUNCHER", "Starting IdoRacing Unified Supervisor v%s", Version)
	logger.Info("LAUNCHER", "Config: %s | Log: %s", configPath, logPath)

	SetupAntiHangEnvironment()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal interception for clean teardown
	sigChan := make(chan os.Signal, 1)
	notifyInterruptSignals(sigChan)
	go func() {
		sig := <-sigChan
		logger.Info("LAUNCHER", "Received signal %v, terminating all subsystems...", sig)
		cancel()
	}()

	// 1. Start Moza Shift Lights Bridge Goroutine
	if cfg.MozaLEDs {
		go func() {
			if err := RunMozaBridge(ctx, *flagPort, MozaDefaultUDP, logger); err != nil {
				logger.Warn("MOZA", "Bridge exited: %v", err)
			}
		}()
	}

	// 2. Start Spotter Engine Goroutine
	spotter := NewSpotterEngine(voicesDir, cfgMgr, logger)
	if cfg.Enabled {
		spotter.Start(ctx)
	}

	// 3. Inspect launch args and detect game
	gameInfo := DetectGameFromArgs(args)
	if *flagSim != "" && *flagSim != "auto" {
		gameInfo.Sim = *flagSim
		gameInfo.GameExe = DefaultExeForSim(*flagSim)
	}
	logger.Info("LAUNCHER", "Detected simulator '%s', target game exe: '%s'", gameInfo.Sim, gameInfo.GameExe)

	// 4. Start HUD overlay if enabled
	if cfg.HUDEnabled {
		hudBin := filepath.Join(repoRoot, "recorder", "bin", "idoracing-hud")
		RunHUDOverlay(ctx, hudBin, configPath, logger)
	}

	// 5. Start Wine Recorder (idoracing-recorder.exe)
	recorderExe := os.Getenv("IDORACING_RECORDER")
	if recorderExe == "" {
		recorderExe = filepath.Join(repoRoot, "recorder", "idoracing-recorder.exe")
	}
	if gameInfo.ProtonPath != "" {
		if fi, err := os.Stat(recorderExe); err == nil && !fi.IsDir() {
			magicBuf := make([]byte, 2)
			isPE := true
			if f, err := os.Open(recorderExe); err == nil {
				n, _ := f.Read(magicBuf)
				_ = f.Close()
				if n >= 2 && string(magicBuf[:2]) != "MZ" {
					isPE = false
					fmt.Fprintf(os.Stderr, "idoracing: WARNING: %s is NOT a Windows PE executable (magic: '%s'). Rebuild with: GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w -H=windowsgui' -o idoracing-recorder.exe ., starting ACC without the recorder\n", recorderExe, string(magicBuf[:n]))
				}
			}

			if isPE {
				recCmd := BuildRecorderCommand(ctx, args, gameInfo.ProtonPath, recorderExe, gameInfo.Sim, gameInfo.GameExe, logPath)
				if recCmd != nil {
					recCmd.Stdout = logger.SubsysWriter("RECORDER")
					recCmd.Stderr = logger.SubsysWriter("RECORDER")

					// Pipe recorder stdout to spotter engine
					pr, pw := io.Pipe()
					recCmd.Stdout = io.MultiWriter(logger.SubsysWriter("RECORDER"), pw)
					go func() {
						buf := make([]byte, 1024)
						var lineBuf string
						for {
							n, err := pr.Read(buf)
							if n > 0 {
								lineBuf += string(buf[:n])
								for {
									idx := -1
									for i, c := range lineBuf {
										if c == '\n' {
											idx = i
											break
										}
									}
									if idx == -1 {
										break
									}
									line := lineBuf[:idx]
									lineBuf = lineBuf[idx+1:]
									spotter.HandleLogLine(line)
								}
							}
							if err != nil {
								break
							}
						}
					}()

					launchRecorder := func() {
						if ctx.Err() == nil {
							logger.Info("RECORDER", "Launching idoracing-recorder.exe in Proton...")
							if err := recCmd.Start(); err != nil {
								logger.Error("RECORDER", "Failed to launch recorder: %v", err)
							} else {
								go func() {
									_ = recCmd.Wait()
									logger.Info("RECORDER", "idoracing-recorder.exe exited")
								}()
							}
						}
					}

					delaySec := 8
					if v := os.Getenv("IDORACING_RECORDER_DELAY"); v != "" {
						if d, err := strconv.Atoi(v); err == nil && d >= 0 {
							delaySec = d
						}
					}

					if delaySec == 0 {
						launchRecorder()
					} else {
						time.AfterFunc(time.Duration(delaySec)*time.Second, launchRecorder)
					}
				}
			}
		}
	}

	// 6. Execute Game Process in foreground
	logger.Info("GAME", "Executing: %s", strings.Join(args, " "))
	gameCmd := exec.CommandContext(ctx, args[0], args[1:]...)
	gameCmd.Stdin = os.Stdin
	gameCmd.Stdout = os.Stdout
	gameCmd.Stderr = os.Stderr

	exitCode := 0
	if err := gameCmd.Start(); err != nil {
		logger.Error("GAME", "Failed to start game process: %v", err)
		cancel()
		os.Exit(1)
	}

	logger.Info("GAME", "Game running (PID: %d)", gameCmd.Process.Pid)
	if err := gameCmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
		logger.Info("GAME", "Game exited with code %d (%v)", exitCode, err)
	} else {
		logger.Info("GAME", "Game exited cleanly (code 0)")
	}

	time.Sleep(150 * time.Millisecond)

	// Teardown all subsystems
	cancel()
	time.Sleep(50 * time.Millisecond)
	logger.Info("LAUNCHER", "All subsystems stopped cleanly. Exiting with code %d", exitCode)
	os.Exit(exitCode)
}
