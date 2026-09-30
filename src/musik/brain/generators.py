"""Playlist generators: Daily / Radio / Weekly / Mood using the shared ranker."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import date
from typing import Any

import numpy as np

from musik.brain.explain import explain_daily, explain_mood, explain_radio, explain_weekly
from musik.brain.ranker import mmr_select, rank_index
from musik.brain.store import save_playlist
from musik.config import get_settings
from musik.index.brute import EmbeddingIndex, load_index
from musik.listen.profile import resolve_taste


@dataclass
class PlaylistBuild:
    kind: str
    name: str
    entries: list[dict[str, Any]] = field(default_factory=list)
    meta: dict[str, Any] = field(default_factory=dict)
    playlist_id: int | None = None

    def persist(self) -> int:
        self.playlist_id = save_playlist(
            kind=self.kind, name=self.name, entries=self.entries, meta=self.meta
        )
        return self.playlist_id


def _mmr_select(
    index: EmbeddingIndex,
    query_sims: np.ndarray,
    *,
    size: int,
    lambda_: float = 0.7,
    artist_penalty: float = 0.15,
    forbidden: set[int] | None = None,
) -> list[int]:
    """Maximal Marginal Relevance over the embedding index. Returns row indices."""
    scores = [float(query_sims[i]) for i in range(index.size)]
    vectors = [index.matrix[i].tolist() for i in range(index.size)]
    artists = [(index.meta[i].get("artist") or "") for i in range(index.size)]
    return mmr_select(
        scores,
        vectors,
        artists,
        size=size,
        lambda_=lambda_,
        artist_penalty=artist_penalty,
        forbidden=forbidden,
    )


def _taste_vector(index: EmbeddingIndex) -> tuple[np.ndarray, str]:
    return resolve_taste(index)


def _forbidden_rows(
    index: EmbeddingIndex,
    track_ids: set[int] | None,
) -> set[int]:
    if not track_ids:
        return set()
    return {
        row
        for track_id in track_ids
        if (row := index.row_of(track_id)) is not None
    }


def _noisy_query(center: np.ndarray, rng: np.random.Generator, scale: float) -> np.ndarray:
    noise = rng.normal(0, scale, size=center.shape).astype(np.float32)
    query = center + noise
    return query / (np.linalg.norm(query) + 1e-12)


def _entries_from_ranked(ranked: list[dict[str, Any]], explain) -> list[dict[str, Any]]:
    return [
        {
            "track_id": int(item["track_id"]),
            "explanation": explain(item),
        }
        for item in ranked
    ]


def generate_daily(
    *,
    size: int = 30,
    seed: int | None = None,
    explore_ratio: float | None = None,
    forbidden_track_ids: set[int] | None = None,
) -> PlaylistBuild:
    """Daily: shared linear ranker + source quotas + MMR."""
    index = load_index()
    if index.size == 0:
        raise RuntimeError("Нет ready-эмбеддингов — сначала musik embed")

    settings = get_settings()
    explore_ratio = settings.explore_ratio if explore_ratio is None else explore_ratio
    if seed is None:
        seed = int(date.today().strftime("%Y%m%d"))
    rng = np.random.default_rng(seed)
    center, center_src = _taste_vector(index)
    query = _noisy_query(center, rng, 0.02)
    forbidden = _forbidden_rows(index, forbidden_track_ids)
    ranked = rank_index(
        index,
        taste=query,
        forbidden=forbidden,
        size=min(size, max(0, index.size - len(forbidden))),
        explore_ratio=explore_ratio,
    )
    entries = _entries_from_ranked(
        ranked,
        lambda item: explain_daily(
            cosine=float(item["sim_taste"]),
            artist=item.get("artist") or "",
            cluster=item.get("cluster_id"),
            diversify=True,
            explore=item["source"] in {"explore_adjacent", "resurface", "new_in_library", "wildcard"},
        ),
    )
    return PlaylistBuild(
        kind="daily",
        name=f"Daily Mix · {date.today().isoformat()}",
        entries=entries,
        meta={
            "size": len(entries),
            "seed": seed,
            "center": center_src,
            "pack_excluded": len(forbidden),
            "explore_ratio": explore_ratio,
            "method": "ranker+quotas+MMR",
            "sources": [item["source"] for item in ranked],
        },
    )


def generate_radio(
    *,
    seed_track_id: int,
    size: int = 30,
    explore_ratio: float | None = None,
) -> PlaylistBuild:
    """Radio: sequential ranked hops from the seed track."""
    index = load_index()
    if index.size == 0:
        raise RuntimeError("Нет ready-эмбеддингов — сначала musik embed")
    start = index.row_of(seed_track_id)
    if start is None:
        raise KeyError(f"seed track_id={seed_track_id} not in index")

    settings = get_settings()
    explore_ratio = settings.explore_ratio if explore_ratio is None else explore_ratio
    seed_title = index.meta[start].get("title") or str(seed_track_id)
    used = {start}
    order: list[dict[str, Any]] = [
        {
            "track_id": int(index.meta[start]["id"]),
            "row": start,
            "source": "manual",
            "sim_taste": 1.0,
            "sim_current": 1.0,
            "artist": index.meta[start].get("artist") or "",
            "cluster_id": index.meta[start].get("cluster_id"),
        }
    ]
    current = index.matrix[start]
    taste, _ = _taste_vector(index)
    for _ in range(1, size):
        nxt = rank_index(
            index,
            taste=taste,
            current=current,
            forbidden=used,
            size=1,
            explore_ratio=explore_ratio,
            pool=80,
        )
        if not nxt:
            break
        item = nxt[0]
        used.add(int(item["row"]))
        order.append(item)
        current = index.matrix[int(item["row"])]

    entries = []
    prev_title = seed_title
    for hop, item in enumerate(order):
        m = index.meta[int(item["row"])]
        title = m.get("title") or ""
        entries.append(
            {
                "track_id": int(item["track_id"]),
                "explanation": explain_radio(
                    cosine=float(item["sim_current"]) if hop else 1.0,
                    seed_title=prev_title if hop else title,
                    hop=hop,
                    explore=item["source"] in {"explore_adjacent", "wildcard", "resurface", "new_in_library"},
                ),
            }
        )
        prev_title = title
    return PlaylistBuild(
        kind="radio",
        name=f"Radio · {index.meta[start].get('artist') or ''} — {seed_title}",
        entries=entries,
        meta={
            "seed_track_id": seed_track_id,
            "size": len(entries),
            "explore_ratio": explore_ratio,
            "method": "ranker-chain",
        },
    )


def generate_weekly(
    *,
    size: int = 50,
    seed: int | None = None,
    explore_ratio: float | None = None,
    forbidden_track_ids: set[int] | None = None,
) -> PlaylistBuild:
    """Weekly: same ranker as radio, with a weekly seed perturbation."""
    index = load_index()
    if index.size == 0:
        raise RuntimeError("Нет ready-эмбеддингов — сначала musik embed")

    settings = get_settings()
    explore_ratio = settings.explore_ratio if explore_ratio is None else explore_ratio
    if seed is None:
        seed = int(date.today().strftime("%G%V"))
    rng = np.random.default_rng(seed)
    center, center_src = _taste_vector(index)
    query = _noisy_query(center, rng, 0.025)
    forbidden = _forbidden_rows(index, forbidden_track_ids)
    ranked = rank_index(
        index,
        taste=query,
        forbidden=forbidden,
        size=min(size, max(0, index.size - len(forbidden))),
        explore_ratio=explore_ratio,
    )
    entries = _entries_from_ranked(
        ranked,
        lambda item: explain_weekly(
            cosine=float(item["sim_taste"]),
            cluster=item.get("cluster_id"),
            bucket=item["source"],
        ),
    )
    return PlaylistBuild(
        kind="weekly",
        name=f"Weekly Mix · {date.today().isocalendar().week} неделя",
        entries=entries,
        meta={
            "size": len(entries),
            "seed": seed,
            "center": center_src,
            "pack_excluded": len(forbidden),
            "explore_ratio": explore_ratio,
            "method": "ranker+quotas",
        },
    )


def generate_mood(
    *,
    mood: str = "energy",
    size: int = 25,
    explore_ratio: float | None = None,
) -> PlaylistBuild:
    index = load_index()
    if index.size == 0:
        raise RuntimeError("Нет ready-эмбеддингов — сначала musik embed")

    settings = get_settings()
    explore_ratio = settings.explore_ratio if explore_ratio is None else explore_ratio
    mood = mood.lower().strip()
    center = index.centroid()
    scores = np.zeros(index.size, dtype=np.float32)
    has_scalar = False
    for i, m in enumerate(index.meta):
        s = 0.0
        if m.get("bpm") is not None:
            s += float(m["bpm"]) / 140.0
            has_scalar = True
        if m.get("lufs") is not None:
            s += (float(m["lufs"]) + 30.0) / 20.0
            has_scalar = True
        scores[i] = s
    if has_scalar and float(scores.std()) > 1e-6:
        order = np.argsort(-scores) if mood == "energy" else np.argsort(scores)
        pool = [int(i) for i in order[: max(size * 3, size)]]
        local = index.centroid(pool[: min(len(pool), size * 2)])
    else:
        sims_c = index.sims_to_vector(center)
        order = np.argsort(sims_c) if mood == "energy" else np.argsort(-sims_c)
        pool = [int(i) for i in order[: max(size * 3, size)]]
        local = index.centroid(pool[: min(len(pool), size * 2)]) if pool else center
    ranked = rank_index(
        index,
        taste=local,
        size=min(size, index.size),
        explore_ratio=explore_ratio,
    )
    entries = []
    for item in ranked:
        m = index.meta[int(item["row"])]
        exp = explain_mood(
            cosine=float(item["sim_taste"]),
            mood=mood,
            lufs=float(m["lufs"]) if m.get("lufs") is not None else None,
            bpm=float(m["bpm"]) if m.get("bpm") is not None else None,
        )
        if item["source"] in {"explore_adjacent", "wildcard", "resurface", "new_in_library"}:
            exp = "exploration · " + exp
        entries.append({"track_id": int(item["track_id"]), "explanation": exp})
    return PlaylistBuild(
        kind="mood",
        name=f"Mood · {mood}",
        entries=entries,
        meta={
            "mood": mood,
            "size": len(entries),
            "scalars": has_scalar,
            "explore_ratio": explore_ratio,
            "method": "ranker+quotas",
        },
    )
