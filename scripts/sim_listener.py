#!/usr/bin/env python3
"""Deterministic synthetic listener simulator for recommendation regressions."""

from __future__ import annotations

import argparse
import math
import random
import statistics
from dataclasses import dataclass
from typing import Callable


Vector = list[float]
EXPLORE_SOURCES = ("explore_adjacent", "resurface", "new_in_library", "wildcard")
MIN_SOURCE_OUTCOMES = 12
EXPLORE_LO = 0.10
EXPLORE_HI = 0.40


def dot(a: Vector, b: Vector) -> float:
    return sum(x * y for x, y in zip(a, b, strict=True))


def normalize(vector: Vector) -> Vector:
    norm = math.sqrt(dot(vector, vector))
    return [value / norm for value in vector] if norm else vector


def blend(base: Vector, update: Vector, rate: float) -> Vector:
    return normalize([(1 - rate) * a + rate * b for a, b in zip(base, update, strict=True)])


def clamp(value: float, lo: float, hi: float) -> float:
    return min(hi, max(lo, value))


def sample_beta(alpha: float, beta: float, rng: random.Random) -> float:
    a = rng.gammavariate(max(alpha, 0.2), 1.0)
    b = rng.gammavariate(max(beta, 0.2), 1.0)
    return a / (a + b) if a + b else 0.5


def label_source(sim_taste: float) -> str:
    if sim_taste >= 0.45:
        return "exploit"
    if sim_taste >= 0.15:
        return "explore_adjacent"
    return "wildcard"


def linear_score(sim_taste: float, sim_session: float, source: str) -> float:
    score = 0.55 * sim_taste + 0.20 * sim_session
    score += {
        "exploit": 0.04,
        "explore_adjacent": 0.05,
        "wildcard": 0.02,
    }.get(source, 0.0)
    return score


@dataclass(frozen=True)
class Track:
    vector: Vector
    cluster: int


@dataclass(frozen=True)
class Scenario:
    name: str
    target_at: Callable[[int, list[Vector]], list[Vector]]
    disliked_cluster: int | None = None


class Bandit:
    def __init__(self) -> None:
        self.agg = [2.0, 8.0]
        self.sources = {source: [1.0, 1.0] for source in EXPLORE_SOURCES}
        self.counts = {source: 0 for source in EXPLORE_SOURCES}

    def ready(self) -> bool:
        return all(count >= MIN_SOURCE_OUTCOMES for count in self.counts.values())

    def decide(self, rng: random.Random) -> tuple[float, dict[str, float], str]:
        if not self.ready():
            return 0.2, {source: 1 / len(EXPLORE_SOURCES) for source in EXPLORE_SOURCES}, "static"
        sampled = sample_beta(self.agg[0], self.agg[1], rng)
        share = clamp(sampled, EXPLORE_LO, EXPLORE_HI)
        weights = {source: sample_beta(*self.sources[source], rng) for source in EXPLORE_SOURCES}
        total = sum(weights.values()) or 1.0
        return share, {source: weight / total for source, weight in weights.items()}, "thompson"

    def update(self, source: str, success: bool) -> None:
        if source in self.counts:
            self.counts[source] += 1
            arm = self.sources[source]
            arm[0 if success else 1] += 1
            self.agg[0 if success else 1] += 1
        elif source == "exploit":
            self.agg[1 if success else 0] += 1


def catalog(rng: random.Random, *, clusters: int = 4, per_cluster: int = 80, dim: int = 12) -> tuple[list[Track], list[Vector]]:
    centers: list[Vector] = []
    for cluster in range(clusters):
        center = [0.0] * dim
        center[cluster] = 1.0
        centers.append(center)
    tracks: list[Track] = []
    for cluster, center in enumerate(centers):
        for _ in range(per_cluster):
            tracks.append(
                Track(
                    normalize([value + rng.gauss(0, 0.13) for value in center]),
                    cluster,
                )
            )
    return tracks, centers


def scenarios() -> list[Scenario]:
    return [
        Scenario("single_taste", lambda step, centers: [centers[0]]),
        Scenario("two_separated_tastes", lambda step, centers: [centers[0], centers[1]]),
        Scenario(
            "mood_shift",
            lambda step, centers: [centers[0] if step < 100 else centers[2]],
        ),
        Scenario("stable_dislike_cluster", lambda step, centers: [centers[0]], disliked_cluster=3),
    ]


def pick_with_quotas(
    candidates: list[int],
    tracks: list[Track],
    long_taste: Vector,
    session_taste: Vector,
    share: float,
    source_share: dict[str, float],
    seen: set[int],
) -> tuple[int, str]:
    scored: list[tuple[float, str, int]] = []
    for track_id in candidates:
        track = tracks[track_id]
        sim_taste = dot(track.vector, long_taste)
        sim_session = dot(track.vector, session_taste)
        source = label_source(sim_taste)
        score = linear_score(sim_taste, sim_session, source)
        if track_id not in seen:
            score += 0.02
        scored.append((score, source, track_id))
    scored.sort(reverse=True)
    n_explore = max(1, round(6 * share))
    explore = [item for item in scored if item[1] in EXPLORE_SOURCES][:n_explore]
    exploit = [item for item in scored if item[1] not in EXPLORE_SOURCES]
    pool = exploit[: max(1, 6 - len(explore))] + explore
    if not pool:
        pool = scored[:1]
    prefer = max(source_share, key=source_share.get) if source_share else "explore_adjacent"
    for item in pool:
        if item[1] == prefer:
            return item[2], item[1]
    return pool[0][2], pool[0][1]


def run(seed: int, scenario: Scenario, *, steps: int = 200) -> dict[str, float]:
    rng = random.Random(seed)
    tracks, centers = catalog(rng)
    long_taste = normalize([a + b for a, b in zip(centers[0], centers[1], strict=True)])
    session_taste = list(long_taste)
    negative: Vector | None = None
    accepted = 0
    disliked_exposure = 0
    quality: list[float] = []
    seen: set[int] = set()
    bandit = Bandit()
    explore_shares: list[float] = []
    explore_picks = 0

    for step in range(steps):
        targets = scenario.target_at(step, centers)
        candidates = list(range(len(tracks)))
        rng.shuffle(candidates)
        candidates = candidates[:64]
        share, source_share, _reason = bandit.decide(rng)
        explore_shares.append(share)

        if scenario.disliked_cluster is not None and step < 8:
            disliked = [
                track_id
                for track_id in candidates
                if tracks[track_id].cluster == scenario.disliked_cluster
            ]
            picked = disliked[0] if disliked else candidates[0]
            source = label_source(dot(tracks[picked].vector, long_taste))
        else:
            picked, source = pick_with_quotas(
                candidates, tracks, long_taste, session_taste, share, source_share, seen
            )
        seen.add(picked)
        track = tracks[picked]
        if source in EXPLORE_SOURCES:
            explore_picks += 1
        affinity = max(dot(track.vector, target) for target in targets)
        is_disliked = scenario.disliked_cluster == track.cluster
        if is_disliked:
            disliked_exposure += 1
            negative = track.vector if negative is None else blend(negative, track.vector, 0.25)
            session_taste = blend(session_taste, normalize([-value for value in track.vector]), 0.08)
            quality.append(0.0)
            bandit.update(source, False)
            continue

        probability = min(0.98, max(0.02, 0.5 + 0.5 * affinity))
        liked = rng.random() < probability
        quality.append(max(0.0, affinity))
        bandit.update(source, liked)
        if liked:
            accepted += 1
            long_taste = blend(long_taste, track.vector, 0.025)
            session_taste = blend(session_taste, track.vector, 0.18)
        else:
            session_taste = blend(session_taste, normalize([-value for value in track.vector]), 0.04)

    return {
        "acceptance": accepted / steps,
        "quality": statistics.fmean(quality),
        "disliked_exposure": disliked_exposure / steps,
        "explore_share": statistics.fmean(explore_shares),
        "explore_pick_rate": explore_picks / steps,
        "explore_min": min(explore_shares),
        "explore_max": max(explore_shares),
    }


def summary(values: list[float]) -> str:
    ordered = sorted(values)
    median = statistics.median(ordered)
    spread = ordered[-1] - ordered[0]
    return f"median={median:.4f} spread={spread:.4f} range=[{ordered[0]:.4f},{ordered[-1]:.4f}]"


def simulate(seeds: list[int], *, steps: int = 200) -> str:
    lines = [f"listener simulator seeds={','.join(map(str, seeds))} steps={steps}"]
    for scenario in scenarios():
        results = [run(seed, scenario, steps=steps) for seed in seeds]
        lines.append(
            f"{scenario.name}: "
            f"acceptance {summary([result['acceptance'] for result in results])}; "
            f"quality {summary([result['quality'] for result in results])}; "
            f"disliked_exposure {summary([result['disliked_exposure'] for result in results])}; "
            f"explore_share {summary([result['explore_share'] for result in results])}"
        )
    accept_then_reject = run_bandit_response(seeds[0], steps=max(80, steps // 2))
    lines.append(
        "bandit_bounds: "
        f"accept_share={accept_then_reject['accept']:.4f} "
        f"reject_share={accept_then_reject['reject']:.4f} "
        f"bounds=[{EXPLORE_LO:.2f},{EXPLORE_HI:.2f}] "
        f"within_bounds={accept_then_reject['within_bounds']}"
    )
    return "\n".join(lines)


def run_bandit_response(seed: int, *, steps: int) -> dict[str, float]:
    rng = random.Random(seed)
    bandit = Bandit()
    for source in EXPLORE_SOURCES:
        bandit.counts[source] = MIN_SOURCE_OUTCOMES
    accepts: list[float] = []
    rejects: list[float] = []
    for _ in range(steps):
        share, _, _ = bandit.decide(rng)
        accepts.append(share)
        bandit.update("explore_adjacent", True)
    for _ in range(steps):
        share, _, _ = bandit.decide(rng)
        rejects.append(share)
        bandit.update("explore_adjacent", False)
    return {
        "accept": statistics.fmean(accepts[-20:]),
        "reject": statistics.fmean(rejects[-20:]),
        "within_bounds": EXPLORE_LO <= min(accepts + rejects) and max(accepts + rejects) <= EXPLORE_HI,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--seeds", default="11,23,37,53,71")
    parser.add_argument("--steps", type=int, default=200)
    args = parser.parse_args()
    seeds = [int(value) for value in args.seeds.split(",") if value.strip()]
    if not seeds:
        parser.error("--seeds must contain at least one integer")
    if args.steps < 1:
        parser.error("--steps must be positive")
    print(simulate(seeds, steps=args.steps))


if __name__ == "__main__":
    main()
