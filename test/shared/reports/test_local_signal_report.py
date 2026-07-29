import importlib.util
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("local_signal_report.py")
SPEC = importlib.util.spec_from_file_location("local_signal_report", MODULE_PATH)
REPORT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(REPORT)


class LocalSignalReportTest(unittest.TestCase):
    def test_attack_signal_uses_current_scenario_label(self):
        signal = {
            "id": "signal-1",
            "name": "payload_dropped",
            "where": "SIGNAL_WHERE_ENDPOINT",
            "labels": {"scenario": "apt-staged-drop-owned-vm"},
        }

        self.assertTrue(REPORT.is_attack_signal(signal, "apt-staged-drop-owned-vm"))
        self.assertEqual(
            REPORT.linked_signal(signal, {}, [])["scenario"],
            "apt-staged-drop-owned-vm",
        )

    def test_fully_resolved_multi_event_signal_requires_every_event(self):
        check = getattr(REPORT, "is_fully_resolved_multi_event", lambda _: False)
        self.assertTrue(check({"event_refs": ["a", "b"], "events": [{}, {}]}))
        self.assertFalse(check({"event_refs": ["a", "b"], "events": [{}]}))


if __name__ == "__main__":
    unittest.main()
