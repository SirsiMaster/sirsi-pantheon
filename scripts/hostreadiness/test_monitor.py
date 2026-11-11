"""python3 scripts/hostreadiness/test_monitor.py — retention of the host-readiness observer."""
import importlib.util
import os
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    "monitor", os.path.join(os.path.dirname(os.path.abspath(__file__)), "unattended-availability-monitor.py"))
monitor = importlib.util.module_from_spec(spec)
spec.loader.exec_module(monitor)


def receipt(state, gui="unknown", at="t"):
    return {"captured_at": at, "admission": {"state": state}, "gui_session": {"gui_state": gui}}


class HistoryTests(unittest.TestCase):
    def setUp(self):
        self.root = tempfile.mkdtemp()
        self.current = os.path.join(self.root, "current.json")

    def runs(self):
        d = os.path.join(self.root, "receipts")
        return sorted(os.listdir(d)) if os.path.isdir(d) else []

    def publish(self, r, n, history="changes"):
        monitor.publish_receipt(r, os.path.join(self.root, "receipts", str(n)), self.current, history)

    def test_changes_keeps_only_transitions_and_always_updates_current(self):
        self.publish(receipt("available", at="1"), 1)
        self.publish(receipt("available", at="2"), 2)
        self.publish(receipt("transport-unavailable", at="3"), 3)
        self.publish(receipt("transport-unavailable", at="4"), 4)
        self.publish(receipt("transport-unavailable", gui="locked", at="5"), 5)
        self.assertEqual(self.runs(), ["1", "3", "5"])
        with open(self.current, encoding="utf-8") as f:
            self.assertIn('"captured_at": "5"', f.read())

    def test_all_keeps_every_run(self):
        for n in range(3):
            self.publish(receipt("available"), n, history="all")
        self.assertEqual(self.runs(), ["0", "1", "2"])

    def test_unreadable_current_counts_as_a_change(self):
        with open(self.current, "w", encoding="utf-8") as f:
            f.write("not json")
        self.publish(receipt("available"), 1)
        self.assertEqual(self.runs(), ["1"])


if __name__ == "__main__":
    unittest.main()
