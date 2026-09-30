from __future__ import annotations

import subprocess
import sys
from pathlib import Path


def test_listener_simulator_is_deterministic_and_covers_scenarios() -> None:
    script = Path(__file__).parents[1] / "scripts" / "sim_listener.py"
    command = [sys.executable, str(script), "--seeds", "3,5,7", "--steps", "40"]
    first = subprocess.run(command, check=True, capture_output=True, text=True).stdout
    second = subprocess.run(command, check=True, capture_output=True, text=True).stdout
    assert first == second
    for scenario in (
        "single_taste",
        "two_separated_tastes",
        "mood_shift",
        "stable_dislike_cluster",
    ):
        assert scenario in first
    assert "median=" in first
    assert "spread=" in first
    assert "explore_share" in first
    assert "bandit_bounds:" in first
    assert "within_bounds=True" in first
