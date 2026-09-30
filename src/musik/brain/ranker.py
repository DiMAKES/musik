"""Shared feature schema, linear score, quotas and MMR for Go↔Python parity."""

from __future__ import annotations

import json
import math
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable

FEATURE_SCHEMA_VERSION = 1
FEATURE_ORDER = [
    "sim_taste",
    "sim_current",
    "sim_session",
    "sim_daypart",
    "sim_mood",
    "sim_place",
    "sim_activity",
    "sim_centroid_max",
    "transition_weight",
    "transition_score",
    "bpm_distance",
    "bpm_missing",
    "lufs_diff",
    "lufs_missing",
    "key_compat",
    "energy_diff",
    "artist_repeat",
    "album_repeat",
    "new_boost",
    "source_exploit",
    "source_transition",
    "source_explore_adjacent",
    "source_resurface",
    "source_new_in_library",
    "source_wildcard",
    "plays_log",
    "early_skip_rate",
    "hours_since_played",
    "never_played",
    "negative_penalty",
]

SOURCE_PRIORITY = [
    "exploit",
    "transition",
    "explore_adjacent",
    "resurface",
    "new_in_library",
    "wildcard",
]

DEFAULT_QUOTAS = {
    "exploit": (2, 4),
    "transition": (0, 2),
    "explore_adjacent": (1, 2),
    "resurface": (0, 1),
    "new_in_library": (0, 1),
    "wildcard": (0, 1),
}

DEFAULT_MODEL_PATH = (
    Path(__file__).resolve().parents[3]
    / "player"
    / "internal"
    / "queue"
    / "model_default.json"
)


@dataclass(frozen=True)
class Model:
    schema_version: int
    model_version: str
    feature_order: list[str]
    mean: list[float]
    std: list[float]
    weights: list[float]
    bias: float
    score_min: float
    score_max: float

    def validate(self) -> None:
        if self.schema_version != FEATURE_SCHEMA_VERSION:
            raise ValueError("model schema_version does not match feature schema")
        if self.feature_order != FEATURE_ORDER:
            raise ValueError("feature_order mismatch")
        n = len(FEATURE_ORDER)
        if not (len(self.weights) == len(self.mean) == len(self.std) == n):
            raise ValueError("model vector lengths must match feature_order")
        if any(std == 0 for std in self.std):
            raise ValueError("std must be non-zero")
        if self.score_max <= self.score_min:
            raise ValueError("score bounds invalid")

    def score(self, values: list[float]) -> float:
        if len(values) != len(self.weights):
            return self.bias
        total = self.bias
        for value, weight, mean, std in zip(values, self.weights, self.mean, self.std, strict=True):
            total += weight * (value - mean) / std
        return min(self.score_max, max(self.score_min, total))


def load_model(path: Path | None = None) -> Model:
    raw = json.loads((path or DEFAULT_MODEL_PATH).read_text())
    model = Model(
        schema_version=int(raw["schema_version"]),
        model_version=str(raw["model_version"]),
        feature_order=list(raw["feature_order"]),
        mean=[float(x) for x in raw["mean"]],
        std=[float(x) for x in raw["std"]],
        weights=[float(x) for x in raw["weights"]],
        bias=float(raw["bias"]),
        score_min=float(raw["score_min"]),
        score_max=float(raw["score_max"]),
    )
    model.validate()
    return model


EXPLORE_SOURCES = (
    "explore_adjacent",
    "resurface",
    "new_in_library",
    "wildcard",
)


def primary_source(sources: Iterable[str]) -> str:
    have = set(sources)
    for source in SOURCE_PRIORITY:
        if source in have:
            return source
    return "wildcard"


def label_sources(
    *,
    sim_taste: float,
    sim_current: float = 0.0,
    sim_centroid_max: float = 0.0,
    plays: int = 0,
    finishes: int = 0,
    hours_since_played: float | None = None,
    hours_since_created: float | None = None,
) -> list[str]:
    out: list[str] = []
    if sim_taste >= 0.45 or sim_centroid_max >= 0.5:
        out.append("exploit")
    if sim_current >= 0.55:
        out.append("transition")
    if 0.15 <= sim_taste < 0.45:
        out.append("explore_adjacent")
    if hours_since_played is not None and hours_since_played >= 14 * 24 and finishes > 0:
        out.append("resurface")
    if hours_since_created is not None and hours_since_created <= 30 * 24 and plays < 2:
        out.append("new_in_library")
    if sim_taste < 0.15:
        out.append("wildcard")
    return out or ["wildcard"]


def feature_vector(
    *,
    sim_taste: float = 0.0,
    sim_current: float = 0.0,
    sim_session: float = 0.0,
    sim_daypart: float = 0.0,
    sim_mood: float = 0.0,
    sim_place: float = 0.0,
    sim_activity: float = 0.0,
    sim_centroid_max: float = 0.0,
    transition_weight: float = 0.0,
    sources: Iterable[str] | None = None,
    bpm_missing: float = 1.0,
    lufs_missing: float = 1.0,
    new_boost: float = 0.0,
    plays: int = 0,
    early_skip_rate: float = 0.0,
    hours_since_played: float = 0.0,
    never_played: float = 1.0,
    negative_penalty: float = 0.0,
) -> list[float]:
    flags = {source: 0.0 for source in SOURCE_PRIORITY}
    for source in sources or ():
        if source in flags:
            flags[source] = 1.0
    values = {
        "sim_taste": sim_taste,
        "sim_current": sim_current,
        "sim_session": sim_session,
        "sim_daypart": sim_daypart,
        "sim_mood": sim_mood,
        "sim_place": sim_place,
        "sim_activity": sim_activity,
        "sim_centroid_max": sim_centroid_max,
        "transition_weight": transition_weight,
        "transition_score": 0.0,
        "bpm_distance": 0.0,
        "bpm_missing": bpm_missing,
        "lufs_diff": 0.0,
        "lufs_missing": lufs_missing,
        "key_compat": 0.0,
        "energy_diff": 0.0,
        "artist_repeat": 0.0,
        "album_repeat": 0.0,
        "new_boost": new_boost,
        "source_exploit": flags["exploit"],
        "source_transition": flags["transition"],
        "source_explore_adjacent": flags["explore_adjacent"],
        "source_resurface": flags["resurface"],
        "source_new_in_library": flags["new_in_library"],
        "source_wildcard": flags["wildcard"],
        "plays_log": math.log1p(plays),
        "early_skip_rate": early_skip_rate,
        "hours_since_played": hours_since_played,
        "never_played": never_played,
        "negative_penalty": negative_penalty,
    }
    return [float(values[name]) for name in FEATURE_ORDER]


def scale_quotas(
    size: int,
    base: dict[str, tuple[int, int]] | None = None,
    explore_ratio: float | None = None,
) -> dict[str, tuple[int, int]]:
    size = max(size, 1)
    base = base or DEFAULT_QUOTAS
    if explore_ratio is not None and explore_ratio <= 0:
        out = {source: (lo, hi) for source, (lo, hi) in base.items()}
        for source in EXPLORE_SOURCES:
            out[source] = (0, 0)
        out["exploit"] = (max(1, size - 1), size)
        return out
    scale = size / 6
    out: dict[str, tuple[int, int]] = {}
    mins = 0
    for source in SOURCE_PRIORITY:
        lo, hi = base[source]
        min_n = int(math.floor(lo * scale + 1e-9))
        max_n = int(round(hi * scale))
        if lo > 0 and min_n < 1 and size >= 3:
            min_n = 1
        if max_n < min_n:
            max_n = min_n
        out[source] = (min_n, max_n)
        mins += min_n
    for source in reversed(SOURCE_PRIORITY):
        if mins <= size:
            break
        lo, hi = out[source]
        if lo > 0:
            out[source] = (lo - 1, hi)
            mins -= 1
    return out


def mmr_select(
    scores: list[float],
    vectors: list[list[float]],
    artists: list[str],
    *,
    size: int,
    lambda_: float = 0.7,
    artist_penalty: float = 0.15,
    forbidden: Iterable[int] | None = None,
) -> list[int]:
    remaining = set(range(len(scores)))
    if forbidden:
        remaining -= set(forbidden)
    selected: list[int] = []
    counts: dict[str, int] = {}
    while remaining and len(selected) < size:
        best_i = None
        best = -1e9
        for i in remaining:
            div = 0.0
            if selected:
                div = max(_dot(vectors[i], vectors[j]) for j in selected)
            artist = (artists[i] or "").strip().lower()
            value = lambda_ * scores[i] - (1.0 - lambda_) * div - artist_penalty * counts.get(artist, 0)
            if value > best:
                best = value
                best_i = i
        if best_i is None:
            break
        selected.append(best_i)
        remaining.remove(best_i)
        artist = (artists[best_i] or "").strip().lower()
        counts[artist] = counts.get(artist, 0) + 1
    return selected


def apply_quotas(
    items: list[dict[str, Any]],
    *,
    size: int,
    quotas: dict[str, tuple[int, int]] | None = None,
) -> list[dict[str, Any]]:
    quotas = quotas or scale_quotas(size)
    by_source: dict[str, list[dict[str, Any]]] = {src: [] for src in SOURCE_PRIORITY}
    for item in sorted(items, key=lambda x: (-float(x["score"]), int(x.get("track_id", 0)))):
        by_source.setdefault(item["source"], []).append(item)
    chosen: list[dict[str, Any]] = []
    used: set[int] = set()
    counts: dict[str, int] = {src: 0 for src in SOURCE_PRIORITY}
    for source in SOURCE_PRIORITY:
        want = quotas.get(source, (0, 0))[0]
        for item in by_source.get(source, []):
            if len(chosen) >= size or counts[source] >= want:
                break
            tid = int(item["track_id"])
            if tid in used:
                continue
            chosen.append(item)
            used.add(tid)
            counts[source] += 1
    for item in sorted(items, key=lambda x: (-float(x["score"]), int(x.get("track_id", 0)))):
        if len(chosen) >= size:
            break
        tid = int(item["track_id"])
        source = item["source"]
        if tid in used or counts.get(source, 0) >= quotas.get(source, (0, size))[1]:
            continue
        chosen.append(item)
        used.add(tid)
        counts[source] = counts.get(source, 0) + 1
    return chosen


def outcome_transition_delta(outcome: str, manual: bool) -> float:
    delta = {"finished": 1.0, "partial": 0.3, "early_skip": -0.1}.get(outcome, 0.0)
    if manual:
        delta *= 1.5
    return delta


def _dot(a: list[float], b: list[float]) -> float:
    return float(sum(x * y for x, y in zip(a, b, strict=False)))


def write_model_atomic(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(f".{path.name}.tmp")
    tmp.write_text(json.dumps(payload, indent=2, ensure_ascii=False), encoding="utf-8")
    tmp.replace(path)


def rank_index(
    index: Any,
    *,
    taste: Any,
    current: Any | None = None,
    forbidden: set[int] | None = None,
    size: int,
    explore_ratio: float | None = None,
    model: Model | None = None,
    pool: int = 200,
) -> list[dict[str, Any]]:
    """Score an embedding index with the shared linear model and source quotas."""
    import numpy as np

    if index.size == 0 or size < 1:
        return []
    model = model or load_model()
    forbidden = forbidden or set()
    taste = np.asarray(taste, dtype=np.float32)
    taste_sims = index.sims_to_vector(taste)
    current_sims = index.sims_to_vector(current) if current is not None else taste_sims
    items: list[dict[str, Any]] = []
    order = np.argsort(-np.maximum(taste_sims, current_sims))
    for row in order:
        row_i = int(row)
        if row_i in forbidden:
            continue
        meta = index.meta[row_i]
        track_id = int(meta["id"])
        sim_taste = float(taste_sims[row_i])
        sim_current = float(current_sims[row_i])
        sources = label_sources(sim_taste=sim_taste, sim_current=sim_current)
        features = feature_vector(
            sim_taste=sim_taste,
            sim_current=sim_current,
            sources=sources,
            bpm_missing=0.0 if meta.get("bpm") is not None else 1.0,
            lufs_missing=0.0 if meta.get("lufs") is not None else 1.0,
        )
        source = primary_source(sources)
        items.append(
            {
                "track_id": track_id,
                "row": row_i,
                "source": source,
                "sources": sources,
                "score": model.score(features),
                "features": features,
                "sim_taste": sim_taste,
                "sim_current": sim_current,
                "artist": meta.get("artist") or "",
                "vector": index.matrix[row_i].tolist(),
                "cluster_id": meta.get("cluster_id"),
            }
        )
        if len(items) >= pool:
            break
    quotas = scale_quotas(size, explore_ratio=explore_ratio)
    chosen = apply_quotas(items, size=size, quotas=quotas)
    if len(chosen) < size:
        used = {int(item["track_id"]) for item in chosen}
        for item in items:
            if len(chosen) >= size:
                break
            if int(item["track_id"]) in used:
                continue
            chosen.append(item)
            used.add(int(item["track_id"]))
    return chosen[:size]
