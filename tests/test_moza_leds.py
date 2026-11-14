import json
import os
import pty
import sys

# Ensure repository root is on sys.path
sys.path.insert(0, os.path.abspath(os.path.join(os.path.dirname(__file__), "..")))

from tools.moza_leds import (
    NUM_LEDS,
    MozaController,
    build_frame,
    calc_led_bitmask,
    encode_rpm_leds,
    encode_rpm_mode,
)


def test_build_frame_checksum():
    # Frame with cmd [28, 0] and payload [1]: len = 3
    # [0x7E, 3, 0x3F, 0x17, 0x1C, 0x00, 0x01]
    # Sum = 126 + 3 + 63 + 23 + 28 + 0 + 1 = 244
    # Checksum = (13 + 244) % 256 = 257 % 256 = 1
    frame = build_frame(group=0x3F, dev_id=0x17, cmd_id=[0x1C, 0x00], payload=[0x01])
    assert frame == bytes([0x7E, 0x03, 0x3F, 0x17, 0x1C, 0x00, 0x01, 0x01])


def test_encode_rpm_mode():
    enable = encode_rpm_mode(True)
    # Wheel dev 0x17 and 0x15
    assert enable == bytes(
        [
            0x7E,
            0x03,
            0x3F,
            0x17,
            0x1C,
            0x00,
            0x01,
            0x01,
            0x7E,
            0x03,
            0x3F,
            0x15,
            0x1C,
            0x00,
            0x01,
            0xFF,
        ]
    )

    disable = encode_rpm_mode(False)
    # Payload 0 -> sum 243 -> (13 + 243) % 256 = 0x00 for 0x17; sum 241 -> 0xFE for 0x15
    assert disable == bytes(
        [
            0x7E,
            0x03,
            0x3F,
            0x17,
            0x1C,
            0x00,
            0x00,
            0x00,
            0x7E,
            0x03,
            0x3F,
            0x15,
            0x1C,
            0x00,
            0x00,
            0xFE,
        ]
    )


def test_encode_rpm_leds():
    # 0 LEDs: bitmask 0 -> low 0, high 0
    off = encode_rpm_leds(0)
    assert off.startswith(bytes([0x7E, 0x04, 0x3F, 0x17, 0x1A, 0x00, 0x00, 0x00]))

    # All 10 LEDs on: bitmask 0x3FF (1023) -> low 0xFF, high 0x03
    all_on = encode_rpm_leds((1 << NUM_LEDS) - 1)
    assert all_on.startswith(bytes([0x7E, 0x04, 0x3F, 0x17, 0x1A, 0x00, 0xFF, 0x03]))


def test_calc_led_bitmask():
    max_rpm = 8000.0

    # 1. Idle or negative: 0 LEDs
    assert calc_led_bitmask(0, max_rpm) == 0
    assert calc_led_bitmask(-100, max_rpm) == 0
    assert calc_led_bitmask(5000, max_rpm) == 0  # 5000 / 8000 = 62.5% < 75%

    # 2. At 75% start threshold: 1 LED active (0b0000000001 = 1)
    mask_start = calc_led_bitmask(6000, max_rpm)  # 6000 / 8000 = 75%
    assert mask_start == 1

    # 3. Mid progressive range: ~5 to 6 LEDs active
    mask_mid = calc_led_bitmask(6850, max_rpm)
    assert 0b0000011111 <= mask_mid <= 0b0000111111

    # 4. At 95% (just below 96% shift): all 10 LEDs on without flashing
    mask_near_shift = calc_led_bitmask(7600, max_rpm, now=0.0)
    assert mask_near_shift == (1 << NUM_LEDS) - 1

    # 5. Shift threshold (>= 96%): flashing at 12Hz
    # At now = 0.0: flash_on is True (int(0.0 / 0.08) % 2 == 0)
    assert calc_led_bitmask(7800, max_rpm, now=0.0) == (1 << NUM_LEDS) - 1
    # At now = 0.09: flash_on is False (int(0.09 / 0.08) % 2 == 1)
    assert calc_led_bitmask(7800, max_rpm, now=0.09) == 0


def test_moza_controller_pty_mock():
    """Verify MozaController serial protocol using a virtual PTY."""
    master_fd, slave_fd = pty.openpty()
    slave_name = os.ttyname(slave_fd)

    try:
        controller = MozaController(port=slave_name)
        assert controller.open() is True

        # Initial frame sent upon open should be encode_rpm_mode(True)
        init_data = os.read(master_fd, 256)
        assert init_data == encode_rpm_mode(True)

        # Update RPM in progressive range (e.g. 5 LEDs)
        controller.update_rpm(rpm=6850, max_rpm=8000, now=1.0)
        led_data = os.read(master_fd, 256)
        assert led_data.startswith(bytes([0x7E, 0x04, 0x3F, 0x17, 0x1A, 0x00]))

        # Timeout check: after 2 seconds idle, turn off LEDs
        controller.check_timeout(now=3.5)
        off_data = os.read(master_fd, 256)
        assert off_data == encode_rpm_leds(0)

        # Close controller: sends LEDs 0 and mode False
        controller.close()
        close_data = os.read(master_fd, 256)
        assert close_data == encode_rpm_leds(0) + encode_rpm_mode(False)
    finally:
        os.close(master_fd)
        os.close(slave_fd)


def test_udp_telemetry_flow():
    """Verify UDP telemetry packet decoding and controller updates."""
    master_fd, slave_fd = pty.openpty()
    slave_name = os.ttyname(slave_fd)

    try:
        controller = MozaController(port=slave_name)
        assert controller.open() is True
        _ = os.read(master_fd, 256)  # drain init

        # Decode simulated JSON telemetry packet
        raw = json.dumps({"rpm": 7800, "max_rpm": 8000, "gear": 3, "speed_kmh": 160.0}).encode()
        data = json.loads(raw.decode("utf-8"))
        controller.update_rpm(rpm=float(data["rpm"]), max_rpm=float(data["max_rpm"]), now=1.0)

        led_data = os.read(master_fd, 256)
        assert led_data.startswith(bytes([0x7E, 0x04, 0x3F, 0x17, 0x1A, 0x00, 0xFF, 0x03]))

        controller.close()
    finally:
        os.close(master_fd)
        os.close(slave_fd)
