package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type SpotterEngine struct {
	voicesDir string
	cfgMgr    *ConfigManager
	logger    *Logger

	mu           sync.Mutex
	currentTrack string
	queue        chan string
}

func NewSpotterEngine(voicesDir string, cfgMgr *ConfigManager, logger *Logger) *SpotterEngine {
	return &SpotterEngine{
		voicesDir: voicesDir,
		cfgMgr:    cfgMgr,
		logger:    logger,
		queue:     make(chan string, 16),
	}
}

func (s *SpotterEngine) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case clipID := <-s.queue:
				s.playClip(clipID)
			}
		}
	}()
}

func (s *SpotterEngine) HandleLogLine(line string) {
	line = strings.TrimSpace(line)
	if strings.Contains(line, "PEP_TRACK|") {
		parts := strings.Split(line, "PEP_TRACK|")
		if len(parts) > 1 {
			track := strings.TrimSpace(parts[1])
			s.mu.Lock()
			s.currentTrack = track
			s.mu.Unlock()
			s.logger.Info("SPOTTER", "Track set to '%s'", track)
		}
	} else if strings.Contains(line, "PEP_PLAY|") {
		parts := strings.Split(line, "PEP_PLAY|")
		if len(parts) > 1 {
			clipID := strings.TrimSpace(parts[1])
			select {
			case s.queue <- clipID:
			default:
				s.logger.Warn("SPOTTER", "Queue full, dropped audio clip: %s", clipID)
			}
		}
	}
}

func (s *SpotterEngine) playClip(clipID string) {
	cfg := s.cfgMgr.Get()
	if !cfg.Enabled {
		return
	}

	s.mu.Lock()
	track := s.currentTrack
	s.mu.Unlock()

	lang := cfg.Language
	if lang == "" {
		lang = "es"
	}

	targetOgg := s.resolveOgg(track, lang, clipID)
	if targetOgg == "" {
		s.logger.Debug("SPOTTER", "Audio clip not found: %s (track: %s, lang: %s)", clipID, track, lang)
		return
	}

	s.logger.Info("SPOTTER", "Playing clip '%s' (vol: %d%%)", filepath.Base(targetOgg), cfg.Volume)
	s.execPlayer(targetOgg, cfg.Volume)
}

func (s *SpotterEngine) resolveOgg(track, lang, clipID string) string {
	// 1. Check exact track voice
	if track != "" {
		p := filepath.Join(s.voicesDir, "tracks", track, lang, clipID+".ogg")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}

	// 2. Check base voices
	p := filepath.Join(s.voicesDir, lang, clipID+".ogg")
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p
	}

	// 3. Corner fallback: hotspot_<slot> -> manifest.json -> turn_<corner_id>.ogg
	if strings.HasPrefix(clipID, "hotspot_") && track != "" {
		slotStr := strings.TrimPrefix(clipID, "hotspot_")
		slot, _ := strconv.Atoi(slotStr)
		manifestPath := filepath.Join(s.voicesDir, "tracks", track, "manifest.json")
		if data, err := os.ReadFile(manifestPath); err == nil {
			type HotspotEntry struct {
				Slot     int `json:"slot"`
				CornerID int `json:"corner_id"`
			}
			type TrackManifest struct {
				Hotspots []HotspotEntry `json:"hotspots"`
			}
			var m TrackManifest
			if err := json.Unmarshal(data, &m); err == nil {
				for _, h := range m.Hotspots {
					if h.Slot == slot && h.CornerID > 0 {
						turnClip := fmt.Sprintf("turn_%d.ogg", h.CornerID)
						turnPath := filepath.Join(s.voicesDir, lang, turnClip)
						if fi, err := os.Stat(turnPath); err == nil && !fi.IsDir() {
							return turnPath
						}
					}
				}
			}
		}
	}
	return ""
}

func (s *SpotterEngine) execPlayer(oggPath string, volume int) {
	if volume <= 0 {
		return
	}

	// 1. Try pw-play (PipeWire - default modern Linux audio)
	if _, err := exec.LookPath("pw-play"); err == nil {
		volFloat := float64(volume) / 100.0
		volStr := fmt.Sprintf("%.2f", volFloat)
		cmd := exec.Command("pw-play", "--volume", volStr, "--media-role=voice", oggPath)
		if err := cmd.Run(); err == nil {
			return
		}
	}

	// 2. Try paplay (PulseAudio)
	if _, err := exec.LookPath("paplay"); err == nil {
		paVol := volume * 65536 / 100
		cmd := exec.Command("paplay", fmt.Sprintf("--volume=%d", paVol), "--property=media.role=voice", oggPath)
		if err := cmd.Run(); err == nil {
			return
		}
	}

	// 3. Fallback: ffplay
	if _, err := exec.LookPath("ffplay"); err == nil {
		cmd := exec.Command("ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet", "-volume", strconv.Itoa(volume), oggPath)
		_ = cmd.Run()
	}
}
