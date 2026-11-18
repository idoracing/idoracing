# IdoRacing Analytics 🏎️

> ### **Every lap tells a story.**
> **Multi-sim telemetry analysis, real-time pit-wall spotter & native zero-overhead launcher. Built for faster laps.**

[![CI](https://github.com/miquelbar/idoracing/actions/workflows/ci.yml/badge.svg)](https://github.com/miquelbar/idoracing/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/miquelbar/idoracing?logo=github&label=Release)](https://github.com/miquelbar/idoracing/releases/latest)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)
[![Go: 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8.svg?logo=go)](https://go.dev/)
[![Python: 3.11+](https://img.shields.io/badge/python-3.11+-blue.svg)](https://www.python.org/)
[![React: 19](https://img.shields.io/badge/react-19-cyan.svg)](https://react.dev/)
[![TypeScript: 6](https://img.shields.io/badge/typescript-6-blue.svg)](https://www.typescriptlang.org/)
[![Ollama: Ready](https://img.shields.io/badge/ollama-qwen3.8%3A27b-orange.svg)](https://ollama.ai/)

**IdoRacing Analytics** is an open-source telemetry platform, live in-game spotter, and AI race engineer for sim racers. Designed with a **two-tier architecture**:
1. **On Track (Zero-Overhead Race Mode)**: Run **100% compiled native binaries in Go** (`idoracing-launcher`, `idoracing-recorder.exe`, `idoracing-hud`). **No Python, no web server, no browser, and zero memory footprint while driving.** Pure multithreaded telemetry recording at 50Hz/100Hz, real-time audio spotter radio on your headset, native X11 pedal oscilloscope overlay, and direct CDC-ACM serial shift lights for Moza steering wheels.
2. **In the Pits (Post-Session Debrief Mode)**: Spin up the Python + React 19 analytics dashboard to inspect 50Hz channel overlays, corner phase breakdowns against **👻 Bubota** (the ideal physics ghost), 2D optimal racing lines, garage setup tuning, and get plain-spoken debriefs from **Pep**, your AI race engineer.

![IdoRacing Analytics Interactive Walkthrough](docs/screenshots/idoracing-demo.gif)

### Supported Simulators
- **Assetto Corsa Competizione (ACC)** — 50Hz shared memory physics, setups, and native MoTeC `.ld`
- **Le Mans Ultimate (LMU)** — 50Hz `$rFactor2SMMP$` shared memory, XML results, and DAMPlugin `.ld`
- **Assetto Corsa 1 (AC) & Assetto Corsa Rally (AC Rally)** — 50Hz shared memory, setups (`.ini`), and point-to-point stages
- **Assetto Corsa EVO (AC EVO - Early Access v0.9)** — 50Hz shared memory, binary `.carsetup` garage setups, and MoTeC `.ld`
- **DiRT Rally 2.0 & EA SPORTS WRC** — 100Hz UDP telemetry, point-to-point linear stage trajectories
- **iRacing** — MoTeC `.ld` & CSV exports
- **Universal 50Hz CSV Logs** — Any logger exporting standard 50Hz CSV telemetry

---

## 💡 The Philosophy

- **Zero Overhead on Track**: When you are behind the wheel, performance and frametime consistency are sacred. You do not need to run Python, a database daemon, or a browser while driving. The unified supervisor (`idoracing-launcher`) and recorder consume `<0.01% CPU` and ~15MB RAM.
- **100% Data Honesty (Zero Hallucinated Metrics)**: Real sessions that only contain lap/sector times (e.g. ACC with `Telemetry Laps = 0` or LMU XML results without a `.ld` file) **never invent fake telemetry traces or fictional corner diagnoses**. The app displays the real timed splits and explains how to activate full 50Hz telemetry for that simulator. Synthetic telemetry is strictly isolated to the explicit **`[ Load Valencia Demo ]`** button.
- **Local & Private by Default**: Your telemetry, sessions, and notes stay in a local SQLite file on your PC. Nothing is sent to the cloud unless you explicitly configure an external LLM API key.
- **Free and Open Source (GPL-3.0)**: Built by sim racers, for sim racers.

### 🏝️ The Names: Why "IdoRacing", "Pep", and "Bubota"?

- **IdoRacing (from *"Idò!"*, not *"I do"*)**: Pronounced `/iˈðɔ/` (stress on the *ò*), **"Idò"** is the quintessential expression from **Mallorca (Balearic Islands, Spain)**. Depending on tone, it means *"Well then!"*, *"So..."*, *"Of course!"*, or *"Let's get down to business!"*. When your race engineer looks at your telemetry trace and spots where those three tenths went, the debrief always starts with: *"Idò, look at your trail braking into Turn 1..."*
- **Pep — Your AI Race Engineer**: **Pep** is a traditional Mallorcan name—and in IdoRacing Analytics, he is your pit-wall race engineer (**P**erformance **E**ngineering **P**artner). Grounded in vehicle dynamics and verified telemetry data, Pep gives direct, actionable feedback without corporate fluff.
- **Bubota — The Ideal Physics Ghost**: In Mallorcan folklore, a ***bubota*** is a phantom or ghost. When comparing two of your own laps (`P1` vs `P2`), you might repeat the exact same braking or line mistake on both. **Bubota** (**B**enchmark **U**ltimate **B**raking & **O**ptimal **T**rajectory **A**nalyzer) is the theoretical physics ghost projecting optimal trail-braking decay (`100% -> 0%` at apex), maximum lateral grip ($a_{y,\max}$), and optimal throttle ramp, highlighting time left on the table even against your Personal Best.

---

## 🏎️ How It Works: Two Modes, Zero Friction

```
                     ┌────────────────────────────────────────────────────────┐
                     │              ON TRACK (RACE MODE)                      │
                     │  100% Native Compiled Binaries · Zero Python/Node      │
                     └──────────────────────────┬─────────────────────────────┘
                                                │
                                    Steam / Proton / Windows
                                                │
                          ┌─────────────────────┴─────────────────────┐
                          ▼                                           ▼
                 [idoracing-launcher]                     [idoracing-recorder.exe]
               Native Go Linux Supervisor                 50Hz Shared Memory / UDP
              (Signals, Audio, Serial, HUD)               (ACC, LMU, AC, ACEVO, DR2)
                          │                                           │
         ┌────────────────┼────────────────┐                          │
         ▼                ▼                ▼                          ▼
   [idoracing-hud]   [Pep Spotter]   [Moza Shift LEDs]      IdoRacing/recordings/
    Native X11      Native pw-play   CDC-ACM Serial 8N1        *.csv telemetry
   Oscilloscope      EBU R128 Voice  10-LED GT3 Palette
  (<0.1% CPU, 15MB) (F9 In-Game Mute) (Boxflat Friendly)
                                                                      │
──────────────────────────────────────────────────────────────────────┼─────────────────
                                                                      │
                     ┌────────────────────────────────────────────────▼────────┐
                     │           IN THE PITS (DEBRIEF MODE)                   │
                     │     Python 3.11+ Backend  ·  React 19 Frontend         │
                     └─────────────────────────────────────────────────────────┘
                                                │
                     ┌──────────────────────────┼──────────────────────────────┐
                     ▼                          ▼                              ▼
             [Telemetry Viewer]         [👻 Bubota Ghost]             [Pep AI Engineer]
            50Hz Channel Overlays      Optimal Physics Envelope     Local Ollama / OpenRouter
            Speed, Pedals, Steer, Gs   Corner Phase Diagnostics     Bilingual Voice & Text
            2D Optimal Racing Line     Lateral Delta (metres)       Interactive Peripheral Tuning
```

---

## 🌟 Key Features

### 1. 🚀 Unified Native Go Supervisor (`idoracing-launcher`)
The standalone native Go supervisor replaces fragile shell scripts and background pipe hacks with a clean, multithreaded binary:
- **Zero Memory Footprint**: Uses Goroutines for non-blocking I/O, terminating in `<2ms` upon game exit via structured Go context cancellation.
- **Structured Millisecond Logging**: Writes console and disk logs with component tags (`[LAUNCHER]`, `[GAME]`, `[RECORDER]`, `[MOZA]`, `[SPOTTER]`, `[HUD]`) for rapid debugging.
- **Auto-Detection**: Identifies running simulator (`acc`, `lmu`, `acevo`, `dr2`, `acrally`, `ac`), automatically builds the Proton command chain, and injects anti-freeze environment variables (`DXVK_ENABLE_NVAPI=0`).
- **Direct Hardware Supervision**: Directly receives telemetry over UDP `10823` and drives Moza steering wheel LEDs over native POSIX serial termios—no Python process required while driving!

### 2. 📊 Live Pedal & Trail Braking HUD Overlay (`idoracing-hud`)
Real-time floating oscilloscope displaying live **Throttle** (Neon Green) and **Brake / Trail Braking** (Racing Red) pedal traces at 60 FPS.

![IdoRacing Live Pedal & Trail Braking HUD](docs/screenshots/live-pedal-hud.png)

- **Pure 100% Compiled Go**: Zero Python, zero Node, zero background web servers needed while racing. Draws directly using X11 double-buffering at `<0.1% CPU` and ~15MB RAM, guaranteeing zero frame drops or frametime jitter in your simulator.
- **Dynamic Trail Braking Detection**: Prominently displays `[>>> TRAIL BRAKING ACTIVE <<<]` in amber when progressive corner-entry brake modulation (bleeding off between 5% and 65% while off throttle) is executed cleanly.
- **Integrated Pit-Wall Radio Controls**:
  - Click `[📻 Pep: Active]` to instantly mute or unmute Pep (`[🔇 Pep: MUTED]`).
  - Adjust spotter radio volume on the fly with `[-]` and `[+]`.
  - Persists immediately to `data/spotter_config.json` and takes effect dynamically during your stint.
- **Interactive & Dismissable**: Click and drag anywhere to position it over your dashboard or second monitor. Dismiss at any time via `[✕]` or pressing `Esc` / `Q`.

### 3. 🛞 Moza Racing Wheel RPM Shift Lights Bridge (Linux / Steam / Proton)
IdoRacing provides first-class support for **MOZA Racing** hardware on Linux without requiring Windows, Moza Pit House, or SimHub.

- **Seamless Boxflat Co-existence**:
  - **[Boxflat](https://github.com/lawstorant/boxflat)** (`boxflat`): Use Boxflat to configure your wheelbase (R3, R5, R9, R12, R16, R21), setup udev rules, set steering lock rotation (e.g. 540°, 900°), calibrate pedals, and tune Force Feedback (FFB) response curves.
  - **IdoRacing Moza RPM Engine**: Talks directly to your steering wheel over CDC-ACM serial (`/dev/ttyACM*`, 115200 baud, Group `0x3F`, device IDs `0x17` and `0x15`) to illuminate the sequential rev LEDs in real time.
- **Compatible Wheels**: Moza CS V2P, GS, KS, ES, FSR, and all Moza steering wheels with CDC-ACM serial LED telemetry.
- **Real-Time 50Hz Shift Indicators**:
  - Progressive 10-LED GT3 RGB palette climbing from 75% to 96% of maximum engine RPM.
  - Flashing shift point alert at 96% RPM so you never miss an upshift.
  - Button backlights preserved at 100% brightness (telemetry isolated strictly to command `[26, 0]`).
- **Built-in Hardware Diagnostics & Simulation Modes**:
  - Hardware status: `./recorder/bin/idoracing-launcher -status`
  - Test sweep: `./recorder/bin/idoracing-launcher -moza-test`
  - Gear-pull simulation: `./recorder/bin/idoracing-launcher -moza-simulate`
- **⚠️ ACC Track Loading Hang Fix (`GE-Proton11-7+`)**:
  If ACC gets stuck indefinitely on the track loading screen with MOZA hardware, switch Steam's Compatibility Tool to **`GE-Proton11-7`** (or newer GE-Proton 11.x) to resolve Wine DirectInput USB enumeration contention.

### 4. 📻 Pep Live Spotter (Zero-Daemon Pit-Wall Radio)
- **EBU R128 Broadcast Loudness Master**: Voice clips are normalized (`loudnorm=I=-11:TP=-0.3:LRA=4`) with speech presence boost (+5dB at 2.8 kHz) and 200 Hz highpass filtering to cut cleanly through GT3 engine roars and transmission whine.
- **Native OS Voice Priority (`media.role=voice`)**: Audio streams through PipeWire (`pw-play --media-role=voice`) or PulseAudio (`paplay`), prioritizing spotter calls over game engine audio.
- **In-Game Mute Hotkey**: Press **`F9`** anytime while driving inside the sim to toggle Pep mute/unmute.
- **Intelligent 3-Phase Cycle**:
  1. *First Stint*: Quiet calibration; reports PB laps and tyre pressure alerts.
  2. *Offline Analysis*: The backend analyzes your laps against **👻 Bubota**, identifies your **Top 3 Hotspot Corners** where you lose the most time, and generates personalized audio cues.
  3. *Subsequent Stints (0% CPU)*: Pep cues you 250m before your problem corners (*"Atento en la curva 4, frena en el 120m y alarga el trail braking"*) and evaluates your apex exit speed.

### 5. 🗺️ Track Map & 50Hz Telemetry Channels (`P1` vs `P2` vs `👻 Bubota`)
- Lap outline coloured by the speed delta between `REF · P1` (Cyan) and `COMP · P2` (Purple).
- **👻 Bubota Curves**: Draws golden dashed `--- 👻 Bubota (Ideal)` limit curves on Speed and Pedals.
- Click any corner on the map or header (`T1`..`T14`) to zoom into its trajectory and phase breakdown.

![IdoRacing Analytics Telemetry Dashboard](docs/screenshots/telemetry-overview.png)

### 6. 🔍 Corner-by-Corner Phase Diagnosis & Optimal Racing Line
- **Phase Breakdown**: Braking Point, Trail Braking Decay, Apex Minimum Speed, Throttle Pickup Point, and Exit Speed.
- **2D Optimal Line Inspector**: Compares apex markers with lateral line offset in metres.

![Corner Optimal Racing Line](docs/screenshots/corner-optimal-line.png)
![Corner Phase Performance Diagnosis](docs/screenshots/corner-phase-diagnosis.png)

### 7. 🗄️ Cross-Session Garage & Normalized Workspace
- **Canonical Normalization**: Maps internal simulator naming (`porsche_992_gt3_r`, `992 GT3 R`) to official titles (**Porsche 911 GT3 R (992)**).
- **Historical Comparison**: Compare today's laps against your all-time **Personal Best** with one click.
- **Garage Setup Inspector**: Deep-links ACC `.json`, AC EVO `.carsetup`, and AC1/Rally `.ini` setups.

![Sessions & Garage Cross-Session Workspace](docs/screenshots/sessions-workspace.png)

---

## 🎮 How to Record Telemetry in Your Sim

### 📦 Download Precompiled Binaries (Latest Release)

Download ready-to-use standalone binaries from the **[Latest Release](https://github.com/miquelbar/idoracing/releases/latest)**:
- 🐧 **[Linux x86_64 (`idoracing-linux-amd64.tar.gz`)](https://github.com/miquelbar/idoracing/releases/latest/download/idoracing-linux-amd64.tar.gz)**: Includes `bin/idoracing-launcher`, `bin/idoracing-hud`, `idoracing-recorder.exe` (for Proton), and offline voice cues.
- 🪟 **[Windows x86_64 (`idoracing-windows-amd64.zip`)](https://github.com/miquelbar/idoracing/releases/latest/download/idoracing-windows-amd64.zip)**: Includes silent background `idoracing-recorder.exe` and `idoracing-launcher.exe`.
- 📱 **[Linux ARM64 (`idoracing-linux-arm64.tar.gz`)](https://github.com/miquelbar/idoracing/releases/latest/download/idoracing-linux-arm64.tar.gz)**: Standalone `idoracing-launcher` for ARM systems.

---

### 🚀 Linux (Steam + Proton) — Universal Setup (Recommended)

1. **Compile the native launcher** (or extract the precompiled archive above):
   ```bash
   cd recorder && go build -trimpath -ldflags="-s -w" -o bin/idoracing-launcher ./cmd/launcher
   ```
2. In Steam, right-click your simulator > **Properties** > **General** > **Launch Options**.
3. Set your launch options:
   ```bash
   gamemoderun /path/to/idoracing/recorder/bin/idoracing-launcher %command%
   ```
   *(Or with MangoHud: `mangohud gamemoderun /path/to/idoracing/recorder/bin/idoracing-launcher %command%`)*  
4. Start the game. The launcher automatically:
   - Identifies the simulator (`acc`, `lmu`, `acevo`, `dr2`, `acrally`, `ac`).
   - Hooks `idoracing-recorder.exe` into the Proton environment.
   - Launches the native pedal HUD overlay (if enabled in `idoracing.json` / `spotter_config.json`).
   - Connects to your Moza wheelbase over serial CDC-ACM.
   - Starts the Pep spotter audio engine.
   - Cleans up every subsystem cleanly when the game exits.

> [!IMPORTANT]
> **Proton Compatibility Recommendations:**
> - **ACC**: Use official **Proton 9.0 (Default)**, **Proton Experimental**, or **GE-Proton 11-7+**. If using MOZA hardware, **GE-Proton 11-7+** fixes track loading screen hangs.
> - **LMU**: Requires **GE-Proton** (for `bcrypt` and DirectInput fixes).
> - **NVAPI Overrides**: Do **NOT** set `PROTON_ENABLE_NVAPI=1` in Steam on NVIDIA GPUs; it causes GPU timeouts and freezes ACC track loading. The launcher automatically forces `DXVK_ENABLE_NVAPI=0`.

---

### 🪟 Windows Setup (All Simulators)
`idoracing-recorder.exe` is compiled as a native background GUI application (`-H=windowsgui`). It runs **completely silently**: no black command prompt popups, no taskbar window, and negligible CPU consumption (<0.01% idle).

- **Option A: Auto-Record on Boot (Recommended — Set & Forget)**:
  1. Press `Win + R`, type `shell:startup`, and press Enter.
  2. Put a shortcut to `idoracing-recorder.exe` in that folder.
  3. It stays asleep in the background and automatically records whenever ACC, LMU, AC, AC EVO, DR2, or WRC starts.
- **Option B: Manual / Desktop Shortcut**:
  Double-click `idoracing-recorder.exe` before driving.
- **Option C: Auto-Exit with Specific Game**:
  ```cmd
  idoracing-recorder.exe -exit-with "AC2-Win64-Shipping.exe,acc.exe"
  idoracing-recorder.exe -sim lmu -exit-with "Le Mans Ultimate.exe,start_protected_game.exe"
  idoracing-recorder.exe -sim ac -exit-with "acs.exe,AssettoCorsa.exe"
  idoracing-recorder.exe -sim acevo -exit-with AssettoCorsaEVO.exe
  idoracing-recorder.exe -sim dr2 -exit-with "dirtrally2.exe,WRC.exe"
  ```

---

## 🏗️ Architecture

```
idoracing/
├── recorder/               # Standalone Go Telemetry Engine (0% Python / 0% Node on track)
│   ├── cmd/launcher/       # Unified native Go supervisor (idoracing-launcher)
│   │   ├── main.go         # Entry point & signal handling (<2ms context shutdown)
│   │   ├── game.go         # Sim detection table & Proton command builder
│   │   ├── moza.go         # Pure Go CDC-ACM serial engine (termios 115200 8N1)
│   │   ├── spotter.go      # Native audio event listener (pw-play / paplay)
│   │   ├── logger.go       # Millisecond structured logger with component tags
│   │   └── config.go       # Unified idoracing.json & spotter_config.json watcher
│   ├── tools/hud/          # Standalone 100% Go X11 Pedal & Trail Braking HUD overlay
│   ├── broadcaster.go      # 50Hz UDP telemetry distributor (Moza: 10823, HUD: 10824)
│   ├── linux/              # Steam Proton shell scripts & automated test suite
│   └── voices/             # Bilingual broadcast-mastered audio clips (es/en)
├── api/                    # FastAPI backend: REST endpoints, live WS, static frontend server
├── frontend/               # React 19 + TypeScript 6 + Vite + Tailwind CSS
│   ├── src/components/     # Telemetry charts, track map, debrief drawer, garage inspector
│   ├── src/views/          # Sessions, telemetry, car setup, engineer radio, settings, live HUD
│   └── src/services/       # Typed API client
├── domain/                 # Pydantic models (Session, Lap, LapComparison, CoachReport)
├── importers/              # MoTeC .ld/.ldx, LMU XML, ACEVO JSON, and 50Hz/100Hz CSV
├── analysis/               # Distance alignment, corner phase diagnostics, Bubota ideal ghost
├── storage/                # SQLite persistence for sessions, setups, and debriefs
├── coach/                  # Ollama / OpenAI LLM clients, Pep radio, deterministic advisor
├── tools/                  # Offline voice generator for spotter (generate_pep_voices.py)
└── tests/                  # Pytest test suite (165+ tests covering physics & hardware)
```

---

## 🚀 Quickstart: Web Dashboard & AI Debrief

When you finish your driving session and want to analyze your data:

### 1. Prerequisites
- Python 3.11+
- Node.js 22+ and npm
- Go 1.22+ (for building recorder & launcher)

### 2. Setup & Launch
```bash
git clone https://github.com/miquelbar/idoracing.git
cd idoracing

# Setup Python backend
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt

# Build Frontend
cd frontend
npm ci
npm run build
cd ..

# Build native Linux launcher & recorder (optional if using prebuilt)
cd recorder
go build -trimpath -ldflags="-s -w" -o bin/idoracing-launcher ./cmd/launcher
cd ..

# Launch API & Web Dashboard
uvicorn api.server:app --host 127.0.0.1 --port 8501
```

Open `http://localhost:8501` in your browser.

### 3. Local AI with Ollama (Optional)
If you want free, 100% local and private AI coaching:
```bash
# Install Ollama (https://ollama.ai)
curl -fsSL https://ollama.ai/install.sh | sh

# Pull a model
ollama pull qwen3.8:27b
```
*(You can also configure OpenAI, OpenRouter, or DeepSeek API keys directly in the web UI under **Settings**).*

---

## 🧪 Testing & Code Quality

IdoRacing features a comprehensive test suite covering vehicle dynamics, hardware protocols, Go concurrency, and UI builds:

```bash
# 1. Python Linting & Unit Tests
.venv/bin/ruff check . && .venv/bin/ruff format --check .
PYTHONPATH=. .venv/bin/pytest tests/

# 2. Go Unit Tests, Launcher Compilation & Windows Cross-Compilation
cd recorder
go test ./...
go build -trimpath -ldflags="-s -w" -o bin/idoracing-launcher ./cmd/launcher
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -H=windowsgui" -o idoracing-recorder.exe .
cd ..

# 3. Frontend Typecheck & Lint
cd frontend && npm run build && npm run lint && cd ..
```

> [!TIP]
> The repository includes `githooks/pre-commit` and `githooks/pre-push` that replicate the complete GitHub Actions CI suite locally to ensure your commits always pass CI cleanly.

---

## 📄 License

This project is licensed under the [GNU General Public License v3.0](LICENSE).

Enjoy faster laps, deeper telemetry insights, and cleaner racing! 🏁
