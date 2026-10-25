import importlib.util
import unittest
from pathlib import Path


GATE_PATH = Path(__file__).with_name("canary-gate.py")
SPEC = importlib.util.spec_from_file_location("canary_gate", GATE_PATH)
gate = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(gate)


class GateDecisionTests(unittest.TestCase):
    def test_small_sample_does_not_promote(self):
        metrics = {"requests": 19, "error_ratio": 0, "p95_seconds": 0.01}
        self.assertEqual(gate.decide(metrics, 10, 0), (10, 0, "insufficient_sample"))

    def test_high_error_rolls_back(self):
        metrics = {"requests": 20, "error_ratio": 0.1, "p95_seconds": 0.01}
        self.assertEqual(gate.decide(metrics, 30, 1), (0, 0, "rollback_threshold_exceeded"))

    def test_requires_multiple_healthy_windows(self):
        metrics = {"requests": 50, "error_ratio": 0, "p95_seconds": 0.05}
        self.assertEqual(gate.decide(metrics, 10, 0), (10, 1, "healthy_window"))
        self.assertEqual(gate.decide(metrics, 10, 1), (20, 0, "promote"))

    def test_promotion_caps_at_100(self):
        metrics = {"requests": 50, "error_ratio": 0, "p95_seconds": 0.05}
        self.assertEqual(gate.decide(metrics, 95, 1), (100, 0, "promote"))


if __name__ == "__main__":
    unittest.main()
