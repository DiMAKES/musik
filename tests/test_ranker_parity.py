from pathlib import Path

from musik.brain.ranker import (
    FEATURE_ORDER,
    DEFAULT_MODEL_PATH,
    load_model,
    mmr_select,
    outcome_transition_delta,
    scale_quotas,
)


def test_default_model_matches_go_artifact() -> None:
    model = load_model()
    assert model.feature_order == FEATURE_ORDER
    assert DEFAULT_MODEL_PATH.exists()
    values = [0.0] * len(FEATURE_ORDER)
    values[FEATURE_ORDER.index("sim_taste")] = 0.8
    values[FEATURE_ORDER.index("sim_current")] = 0.4
    score = model.score(values)
    assert 0.4 < score < 0.8


def test_quota_rounding_for_queue_size_six() -> None:
    quotas = scale_quotas(6)
    assert quotas["exploit"][0] == 2
    assert quotas["explore_adjacent"][0] == 1
    assert sum(lo for lo, _ in quotas.values()) <= 6
    closed = scale_quotas(6, explore_ratio=0)
    assert closed["explore_adjacent"] == (0, 0)
    assert closed["wildcard"] == (0, 0)


def test_mmr_penalizes_same_artist() -> None:
    scores = [1.0, 0.99, 0.5]
    vectors = [[1.0, 0.0], [0.99, 0.01], [0.0, 1.0]]
    artists = ["a", "a", "b"]
    selected = mmr_select(scores, vectors, artists, size=2, lambda_=0.6)
    assert selected[0] == 0
    assert selected[1] == 2


def test_outcome_transition_weights() -> None:
    assert outcome_transition_delta("finished", False) == 1.0
    assert outcome_transition_delta("partial", False) == 0.3
    assert outcome_transition_delta("early_skip", False) == -0.1
    assert outcome_transition_delta("finished", True) == 1.5


def test_golden_feature_order_file() -> None:
    golden = Path(__file__).parent / "golden" / "feature_order_v1.txt"
    assert golden.read_text().splitlines() == FEATURE_ORDER
