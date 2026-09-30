from __future__ import annotations

import numpy as np

from musik.brain.ranker import label_sources, primary_source, rank_index
from musik.index.brute import EmbeddingIndex


def test_source_labels_split_near_and_far():
    assert primary_source(label_sources(sim_taste=0.7)) == "exploit"
    assert "explore_adjacent" in label_sources(sim_taste=0.3)
    assert primary_source(label_sources(sim_taste=0.05)) == "wildcard"


def test_rank_index_respects_forbidden_and_size():
    rng = np.random.default_rng(0)
    n, d = 40, 16
    mat = rng.normal(size=(n, d)).astype(np.float32)
    mat /= np.linalg.norm(mat, axis=1, keepdims=True)
    meta = [
        {"id": i, "artist": f"A{i % 7}", "title": f"T{i}", "path": "", "file_md5": str(i)}
        for i in range(n)
    ]
    idx = EmbeddingIndex(
        track_ids=np.arange(n, dtype=np.int64),
        matrix=mat,
        meta=meta,
        md5s=[str(i) for i in range(n)],
    )
    ranked = rank_index(idx, taste=idx.centroid(), forbidden={0, 1}, size=8, explore_ratio=0.25)
    assert len(ranked) == 8
    assert {item["track_id"] for item in ranked}.isdisjoint({0, 1})
    assert all(item["source"] for item in ranked)
