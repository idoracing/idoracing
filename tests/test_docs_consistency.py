"""
Tests that documentation (README.md, recorder/README.md) is consistently up to date
with all supported simulators, adapters, and launch scripts in the codebase.
"""

from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
README_PATH = ROOT / "README.md"
RECORDER_README_PATH = ROOT / "recorder" / "README.md"
CHANGELOG_PATH = ROOT / "CHANGELOG.md"

CANONICAL_SIMULATORS = [
    "Assetto Corsa Competizione",
    "Le Mans Ultimate",
    "Assetto Corsa",
    "Assetto Corsa Rally",
    "Assetto Corsa EVO",
    "DiRT Rally 2.0",
    "EA SPORTS WRC",
    "iRacing",
]

CANONICAL_BINARIES = [
    "idoracing-launcher",
    "idoracing-recorder.exe",
]


def test_readme_documents_all_supported_simulators():
    assert README_PATH.exists(), "README.md must exist at root"
    readme_text = README_PATH.read_text(encoding="utf-8")

    for sim in CANONICAL_SIMULATORS:
        assert sim in readme_text, f"README.md is missing documentation for supported simulator: '{sim}'"


def test_recorder_readme_documents_all_supported_simulators():
    assert RECORDER_README_PATH.exists(), "recorder/README.md must exist"
    recorder_readme_text = RECORDER_README_PATH.read_text(encoding="utf-8")

    for sim in CANONICAL_SIMULATORS:
        assert sim in recorder_readme_text, (
            f"recorder/README.md is missing documentation for supported simulator: '{sim}'"
        )


def test_readme_documents_canonical_binaries():
    readme_text = README_PATH.read_text(encoding="utf-8")
    for binary in CANONICAL_BINARIES:
        assert binary in readme_text, f"README.md is missing reference to binary: '{binary}'"


def test_recorder_readme_documents_canonical_binaries():
    recorder_readme_text = RECORDER_README_PATH.read_text(encoding="utf-8")
    for binary in CANONICAL_BINARIES:
        assert binary in recorder_readme_text, f"recorder/README.md is missing reference to binary: '{binary}'"


def test_no_legacy_shell_launchers_in_tree():
    """Ensure no legacy .sh launcher wrappers remain in recorder/linux/."""
    legacy_launchers = [
        "sim-with-recorder.sh",
        "acc-with-recorder.sh",
        "lmu-with-recorder.sh",
        "ac-with-recorder.sh",
        "acevo-with-recorder.sh",
        "dr2-with-recorder.sh",
        "idoracing-launch.sh",
    ]
    for script in legacy_launchers:
        p = ROOT / "recorder" / "linux" / script
        assert not p.exists(), f"Legacy launcher shell script {script} must not exist in {p}"


def test_no_already_supported_simulators_in_readme_wishlist():
    readme_text = README_PATH.read_text(encoding="utf-8")
    contrib_section = readme_text.split("## 🤝 Contributing")[-1].split("##")[0]

    for sim in CANONICAL_SIMULATORS:
        assert sim.lower() not in contrib_section.lower(), (
            f"README.md Contributing wishlist contradicts itself by listing already supported simulator: '{sim}'"
        )
