"""compute_audio_params decodes once; the values must match the per-metric helpers."""

from __future__ import annotations

import numpy as np
import pytest
import soundfile as sf

from musik.scanner.audio_params import (
    ANALYSIS_SR,
    compute_audio_params,
    compute_bpm,
    compute_key_mode,
    compute_lufs,
)


def _write_tone(path, *, seconds=20.0, sr=44100):
    """A pulsed tone: loud enough for LUFS, periodic enough for beat tracking."""
    t = np.linspace(0.0, seconds, int(sr * seconds), endpoint=False)
    tone = 0.3 * np.sin(2 * np.pi * 220.0 * t)
    beat = (np.sin(2 * np.pi * 2.0 * t) > 0.0).astype(np.float32)  # 120 BPM
    sf.write(str(path), (tone * (0.4 + 0.6 * beat)).astype(np.float32), sr)
    return path


def test_single_decode_matches_separate_helpers(tmp_path):
    wav = _write_tone(tmp_path / "tone.wav")

    params = compute_audio_params(wav, max_seconds=15.0, key_seconds=10.0)
    lufs = compute_lufs(wav, max_seconds=15.0)
    bpm = compute_bpm(wav, max_seconds=15.0)
    key, mode = compute_key_mode(wav, max_seconds=10.0)

    assert params.lufs == pytest.approx(lufs, abs=1e-6)
    assert params.bpm == pytest.approx(bpm, abs=1e-6)
    assert (params.key, params.mode) == (key, mode)


def test_shorter_key_window_does_not_change_bpm_window(tmp_path):
    wav = _write_tone(tmp_path / "tone.wav", seconds=30.0)

    wide = compute_audio_params(wav, max_seconds=25.0, key_seconds=10.0)
    narrow = compute_audio_params(wav, max_seconds=25.0, key_seconds=25.0)

    assert wide.bpm == pytest.approx(narrow.bpm, abs=1e-6)
    assert wide.lufs == pytest.approx(narrow.lufs, abs=1e-6)


def test_unreadable_file_returns_empty_params(tmp_path):
    broken = tmp_path / "broken.wav"
    broken.write_bytes(b"not audio")

    params = compute_audio_params(broken)

    assert (params.lufs, params.bpm, params.key, params.mode) == (None, None, None, None)


def test_analysis_sr_is_shared():
    assert ANALYSIS_SR == 22_050
