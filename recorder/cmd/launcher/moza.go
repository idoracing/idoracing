package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const (
	MozaFrameHeader   byte    = 0x7E
	MozaMagicChecksum byte    = 13
	MozaGroup         byte    = 0x3F
	MozaNumLEDs               = 10
	MozaStartPct              = 0.75
	MozaShiftPct              = 0.96
	MozaDefaultUDP            = 10823
)

var MozaWheelDeviceIDs = []byte{0x17, 0x15} // Primary wheel ID 23 (0x17), legacy ID 21 (0x15)

// buildMozaFrame constructs a Moza protocol serial frame:
// 0x7E | len | group | dev_id | cmd_id... | payload... | checksum
// where len = len(cmd_id) + len(payload)
// and checksum = (13 + sum(all preceding bytes)) % 256
func buildMozaFrame(group byte, devID byte, cmdID []byte, payload []byte) []byte {
	length := byte(len(cmdID) + len(payload))
	frame := make([]byte, 0, 5+len(cmdID)+len(payload))
	frame = append(frame, MozaFrameHeader, length, group, devID)
	frame = append(frame, cmdID...)
	frame = append(frame, payload...)

	var sum int = int(MozaMagicChecksum)
	for _, b := range frame {
		sum += int(b)
	}
	checksum := byte(sum % 256)
	frame = append(frame, checksum)
	return frame
}

// encodeMozaPalette configures vibrant RGB colors for the 10 RPM shift LEDs (Green -> Yellow -> Red -> Blue)
// and ensures button backlights remain enabled at 100% constant illumination.
func encodeMozaPalette() []byte {
	colors := [10][3]byte{
		{0, 255, 0},    // 0: Green
		{0, 255, 0},    // 1: Green
		{0, 255, 0},    // 2: Green
		{255, 200, 0},  // 3: Yellow
		{255, 200, 0},  // 4: Yellow
		{255, 0, 0},    // 5: Red
		{255, 0, 0},    // 6: Red
		{255, 0, 0},    // 7: Red
		{0, 150, 255},  // 8: Blue
		{0, 150, 255},  // 9: Blue
	}

	cBytes := make([]byte, 0, 40)
	for i := 0; i < 10; i++ {
		cBytes = append(cBytes, byte(i), colors[i][0], colors[i][1], colors[i][2])
	}

	var frames []byte
	for _, dev := range MozaWheelDeviceIDs {
		// 1. Telemetry mode = 1 (external control)
		frames = append(frames, buildMozaFrame(MozaGroup, dev, []byte{0x1C, 0x00}, []byte{1})...)
		// 2. RPM Colors: id [25, 0], two 20-byte chunks
		frames = append(frames, buildMozaFrame(MozaGroup, dev, []byte{0x19, 0x00}, cBytes[:20])...)
		frames = append(frames, buildMozaFrame(MozaGroup, dev, []byte{0x19, 0x00}, cBytes[20:])...)
		// 3. RPM Brightness: id [27, 0, 255], value 100%
		frames = append(frames, buildMozaFrame(MozaGroup, dev, []byte{0x1B, 0x00, 0xFF}, []byte{100})...)
		// 4. Buttons Brightness: id [27, 1, 255], value 100%
		frames = append(frames, buildMozaFrame(MozaGroup, dev, []byte{0x1B, 0x01, 0xFF}, []byte{100})...)
		// 5. Buttons idle effect: id [29, 1], value 1 (Constant illuminated)
		frames = append(frames, buildMozaFrame(MozaGroup, dev, []byte{0x1D, 0x01}, []byte{1})...)
	}
	return frames
}

// encodeMozaRPMLEDs formats the 10-bit RPM LEDs bitmask to command [26, 0].
// It NEVER touches command [26, 1] to preserve hardware button backlighting.
func encodeMozaRPMLEDs(bitmask uint16) []byte {
	low := byte(bitmask & 0xFF)
	high := byte((bitmask >> 8) & 0xFF)
	var frames []byte
	for _, dev := range MozaWheelDeviceIDs {
		frames = append(frames, buildMozaFrame(MozaGroup, dev, []byte{0x1A, 0x00}, []byte{low, high})...)
	}
	return frames
}

// calcMozaBitmask calculates the 10-LED bitmask based on RPM, redline and time.
func calcMozaBitmask(rpm, maxRPM float64, now time.Time) uint16 {
	if maxRPM <= 0 || rpm < (maxRPM*MozaStartPct) {
		return 0
	}

	pct := (rpm - (maxRPM * MozaStartPct)) / (maxRPM * (MozaShiftPct - MozaStartPct))
	if pct >= 1.0 {
		// Shift flash: toggle on/off every ~80ms (12.5 Hz)
		flashOn := (now.UnixMilli()/80)%2 == 0
		if flashOn {
			return 0x03FF
		}
		return 0
	}

	activeLEDs := int(pct*MozaNumLEDs) + 1
	if activeLEDs > MozaNumLEDs {
		activeLEDs = MozaNumLEDs
	}
	if activeLEDs < 1 {
		activeLEDs = 1
	}
	return (1 << activeLEDs) - 1
}

type mozaController struct {
	portPath     string
	file         *os.File
	lastMask     int
	lastPacketAt time.Time
}

func newMozaController(port string) *mozaController {
	return &mozaController{
		portPath: port,
		lastMask: -1,
	}
}

func (c *mozaController) open() bool {
	if c.file != nil {
		return true
	}
	path := c.portPath
	if path == "" {
		path = findMozaSerialPort()
	}
	if path == "" {
		return false
	}

	f, err := openMozaPortFile(path)
	if err != nil {
		return false
	}

	if err := configureMozaSerialPort(f.Fd()); err != nil {
		_ = f.Close()
		return false
	}

	c.file = f
	c.portPath = path
	_, _ = c.file.Write(encodeMozaPalette())
	return true
}

func (c *mozaController) write(data []byte) {
	if c.file != nil && len(data) > 0 {
		_, _ = c.file.Write(data)
	}
}

func (c *mozaController) updateRPM(rpm, maxRPM float64, now time.Time) {
	c.lastPacketAt = now
	if c.file == nil {
		if !c.open() {
			return
		}
	}
	mask := int(calcMozaBitmask(rpm, maxRPM, now))
	if mask != c.lastMask {
		c.write(encodeMozaRPMLEDs(uint16(mask)))
		c.lastMask = mask
	}
}

func (c *mozaController) checkTimeout(now time.Time) {
	if c.lastMask > 0 && now.Sub(c.lastPacketAt) > 1500*time.Millisecond {
		c.write(encodeMozaRPMLEDs(0))
		c.lastMask = 0
	}
}

func (c *mozaController) close() {
	if c.file != nil {
		c.write(encodeMozaRPMLEDs(0))
		_ = c.file.Close()
		c.file = nil
	}
}

// RunMozaBridge is the primary goroutine that listens to UDP telemetry from the recorder and updates the Moza wheel.
func RunMozaBridge(ctx context.Context, port string, udpPort int, logger *Logger) error {
	addr := &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: udpPort,
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("listen UDP %s: %w", addr, err)
	}
	defer conn.Close()

	logger.Info("MOZA", "Listening on UDP 127.0.0.1:%d (ready for telemetry)", udpPort)

	ctrl := newMozaController(port)
	defer ctrl.close()

	buf := make([]byte, 2048)
	type TelemetryPacket struct {
		RPM      float64 `json:"rpm"`
		MaxRPM   float64 `json:"max_rpm"`
		Gear     int     `json:"gear"`
		SpeedKmh float64 `json:"speed_kmh"`
	}

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	// Goroutine to check timeout
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				ctrl.checkTimeout(t)
			}
		}
	}()

	for {
		if ctx.Err() != nil {
			break
		}

		_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			if strings.Contains(err.Error(), "timeout") {
				continue
			}
			if ctx.Err() != nil {
				break
			}
			continue
		}

		var pkt TelemetryPacket
		if err := json.Unmarshal(buf[:n], &pkt); err != nil {
			continue
		}

		if pkt.RPM > 0 && ctrl.file == nil {
			if ctrl.open() {
				logger.Info("MOZA", "Connected to wheel at %s, palette & 100%% brightness initialized", ctrl.portPath)
			}
		}

		if ctrl.file != nil {
			ctrl.updateRPM(pkt.RPM, pkt.MaxRPM, time.Now())
		}
	}

	ctrl.close()
	logger.Info("MOZA", "Bridge stopped, RPM LEDs cleared, buttons preserved")
	return nil
}

// SimulateMozaSweep runs a dynamic gear pull simulation directly on the Moza wheel.
func SimulateMozaSweep(port string, duration time.Duration, logger *Logger) error {
	ctrl := newMozaController(port)
	if !ctrl.open() {
		return fmt.Errorf("cannot connect to Moza device (%s)", ctrl.portPath)
	}
	defer ctrl.close()

	logger.Info("MOZA", "Simulating gear pull (6,000 -> 9,000 RPM) on %s for %v...", ctrl.portPath, duration)
	start := time.Now()
	maxRPM := 9000.0

	for time.Since(start) < duration {
		now := time.Now()
		elapsedSec := float64(now.Sub(start).Milliseconds()%3000) / 1000.0

		var rpm float64
		if elapsedSec < 1.8 {
			// Progressive acceleration 6,000 -> 9,000 RPM
			rpm = 6000.0 + (elapsedSec/1.8)*3000.0
		} else if elapsedSec < 2.4 {
			// Redline shift flash
			rpm = 9000.0
		} else {
			// Next gear shift drop
			rpm = 6200.0
		}

		ctrl.updateRPM(rpm, maxRPM, now)
		time.Sleep(20 * time.Millisecond)
	}

	ctrl.write(encodeMozaRPMLEDs(0))
	logger.Info("MOZA", "Simulation completed successfully (RPM LEDs off, buttons active)")
	return nil
}

// TestMozaLEDs runs an instant test pattern on the Moza wheel.
func TestMozaLEDs(port string, logger *Logger) error {
	ctrl := newMozaController(port)
	if !ctrl.open() {
		return fmt.Errorf("cannot connect to Moza device (%s)", ctrl.portPath)
	}
	defer ctrl.close()

	logger.Info("MOZA", "Testing 10 rev LEDs on %s...", ctrl.portPath)
	for i := 1; i <= MozaNumLEDs; i++ {
		mask := uint16((1 << i) - 1)
		ctrl.write(encodeMozaRPMLEDs(mask))
		time.Sleep(75 * time.Millisecond)
	}
	time.Sleep(250 * time.Millisecond)

	for flash := 0; flash < 4; flash++ {
		ctrl.write(encodeMozaRPMLEDs(0x03FF))
		time.Sleep(80 * time.Millisecond)
		ctrl.write(encodeMozaRPMLEDs(0))
		time.Sleep(80 * time.Millisecond)
	}

	logger.Info("MOZA", "LED test completed successfully")
	return nil
}
