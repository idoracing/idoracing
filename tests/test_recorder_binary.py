"""
Regression tests for recorder executables and Linux launcher script binary validation.
Guarantees that idoracing-recorder.exe is never a Linux ELF binary and that launcher
scripts properly identify Windows PE executables.
"""

from __future__ import annotations

import os
import subprocess
import tempfile
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
RECORDER_DIR = REPO_ROOT / "recorder"


def test_recorder_binaries_are_windows_pe():
    """
    Regression test: idoracing-recorder.exe must be a real Windows PE executable (magic 'MZ'),
    never a Linux ELF binary (magic '\x7fELF') that aborts with 'reads ACC Windows shared memory'.
    """
    candidates = [
        RECORDER_DIR / "idoracing-recorder.exe",
        RECORDER_DIR / "linux" / "idoracing-recorder.exe",
    ]

    for exe in candidates:
        if not exe.exists():
            continue
        data = exe.read_bytes()
        assert len(data) >= 2, f"{exe} is empty"
        magic = data[:2]
        assert magic == b"MZ", (
            f"{exe} is NOT a Windows PE executable! Got magic {magic!r}. "
            f"If it starts with {data[:4]!r}, it is a Linux ELF binary built without GOOS=windows."
        )
        assert not data.startswith(b"\x7fELF"), f"{exe} was accidentally built as a Linux ELF executable!"


def test_launcher_warns_on_elf_executable():
    """
    Test that idoracing-launcher detects an ELF binary configured as the recorder
    and emits a visible warning on stderr rather than failing silently.
    """
    with tempfile.TemporaryDirectory() as tmpdir:
        tmp_path = Path(tmpdir)
        fake_elf = tmp_path / "fake-recorder.exe"
        # Write valid Linux ELF magic bytes followed by dummy data
        fake_elf.write_bytes(b"\x7fELF\x02\x01\x01\x00" + b"\x00" * 64)

        fake_proton = tmp_path / "proton"
        fake_proton.write_text("#!/bin/sh\nexit 0\n")
        fake_proton.chmod(0o755)

        bin_launcher = RECORDER_DIR / "bin" / "idoracing-launcher"
        if not bin_launcher.exists():
            subprocess.run(
                ["go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(bin_launcher), "./cmd/launcher"],
                cwd=RECORDER_DIR,
                check=True,
            )

        cmd = [
            str(bin_launcher),
            str(fake_proton),
            "waitforexitandrun",
            "/games/acc/AC2-Win64-Shipping.exe",
        ]
        env = {
            **os.environ,
            "IDORACING_RECORDER": str(fake_elf),
            "IDORACING_RECORDER_DELAY": "0",
            "IDORACING_RECORDER_LOG": str(tmp_path / "test.log"),
        }

        res = subprocess.run(cmd, env=env, capture_output=True, text=True, timeout=15)
        stderr = res.stderr
        assert "WARNING" in stderr and "Windows PE" in stderr, (
            f"Launcher did not warn about ELF executable! Stderr: {stderr}"
        )


def test_launcher_e2e_game_detection_and_recorder_spawn():
    """
    End-to-end regression test: verifies that idoracing-launcher properly detects
    all simulators from Steam/Proton command line arguments and spawns the Windows
    recorder binary with the exact required simulator and exit-with parameters.
    Replaces the legacy test-sim-launchers.sh test suite.
    """
    bin_launcher = RECORDER_DIR / "bin" / "idoracing-launcher"
    if not bin_launcher.exists():
        subprocess.run(
            ["go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(bin_launcher), "./cmd/launcher"],
            cwd=RECORDER_DIR,
            check=True,
        )

    sim_scenarios = [
        ("acc", "/games/acc/AC2-Win64-Shipping.exe", "-exit-with AC2-Win64-Shipping.exe,acc.exe", False),
        (
            "lmu",
            "/games/lmu/Le Mans Ultimate.exe",
            "-sim lmu -exit-with Le Mans Ultimate.exe,start_protected_game.exe",
            True,
        ),
        ("acrally", "/games/ac/acr.exe", "-sim acrally -exit-with acr.exe,AssettoCorsaRally.exe", True),
        ("acevo", "/games/acevo/AssettoCorsaEVO.exe", "-sim acevo -exit-with AssettoCorsaEVO.exe", True),
        ("ac", "/games/ac/acs.exe", "-sim ac -exit-with acs.exe,AssettoCorsa.exe", True),
        ("dr2", "/games/dr2/dirtrally2.exe", "-sim dr2 -exit-with dirtrally2.exe,WRC.exe", True),
    ]

    with tempfile.TemporaryDirectory() as tmpdir:
        tmp_path = Path(tmpdir)
        calls_log = tmp_path / "calls.log"

        fake_proton = tmp_path / "proton"
        fake_proton.write_text(f'#!/bin/sh\necho "$@" >> "{calls_log}"\nexit 0\n')
        fake_proton.chmod(0o755)

        fake_recorder = tmp_path / "idoracing-recorder.exe"
        fake_recorder.write_bytes(b"MZ\x00\x00" + b"\x00" * 64)

        for sim_name, game_path, expected_args, has_sim_flag in sim_scenarios:
            if calls_log.exists():
                calls_log.unlink()

            cmd = [
                str(bin_launcher),
                str(fake_proton),
                "waitforexitandrun",
                game_path,
            ]
            env = {
                **os.environ,
                "IDORACING_RECORDER": str(fake_recorder),
                "IDORACING_RECORDER_DELAY": "0",
                "IDORACING_RECORDER_LOG": str(tmp_path / "test.log"),
            }

            res = subprocess.run(cmd, env=env, capture_output=True, text=True, timeout=10)
            assert res.returncode == 0, f"Launcher failed on {sim_name}: {res.stderr}"

            calls = calls_log.read_text() if calls_log.exists() else ""
            assert f"waitforexitandrun {game_path}" in calls, f"Game was not launched for {sim_name}: {calls}"
            assert f"run {fake_recorder}" in calls, f"Recorder was not spawned for {sim_name}: {calls}"
            for part in expected_args.split():
                assert part in calls, f"Missing '{part}' in recorder call for {sim_name}: {calls}"
            if not has_sim_flag:
                assert "-sim " not in calls, f"Default {sim_name} should not pass -sim flag: {calls}"
