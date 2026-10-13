import random

from src.workload import patterns


def test_intensity_factor_disabled_without_jitter():
    rng = random.Random(1)
    assert patterns.next_intensity_factor(rng, 1.0, 0.0) == 1.0


def test_intensity_factor_stays_within_bounds():
    rng = random.Random(42)
    value = 1.0
    for _ in range(1000):
        value = patterns.next_intensity_factor(rng, value, 0.5)
        assert 0.5 <= value <= 1.5


def test_intensity_factor_is_reproducible_for_a_seed():
    rng_a = random.Random(7)
    rng_b = random.Random(7)
    a = b = 1.0
    for _ in range(50):
        a = patterns.next_intensity_factor(rng_a, a, 0.6)
        b = patterns.next_intensity_factor(rng_b, b, 0.6)
        assert a == b
