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

    def test_changed_failure_details_are_retained_without_timestamp_noise(self):
        first = receipt("observer-uncertain", at="1")
        first["admission"]["reason"] = "observer limited"
        first["transport"] = {"attempts": [{"captured_at": "1", "ssh": {"state": "unknown", "reason": "network down"}, "screen-sharing": {"state": "timeout"}}]}
        second = receipt("observer-uncertain", at="2")
        second["admission"]["reason"] = "observer limited"
        second["transport"] = {"attempts": [{"captured_at": "2", "ssh": {"state": "unknown", "reason": "network down"}, "screen-sharing": {"state": "timeout"}}]}
        third = receipt("observer-uncertain", at="3")
        third["admission"]["reason"] = "observer limited"
        third["transport"] = {"attempts": [{"captured_at": "3", "ssh": {"state": "unknown", "reason": "network unreachable"}, "screen-sharing": {"state": "timeout"}}]}
        self.publish(first, 1)
        self.publish(second, 2)
        self.publish(third, 3)
        self.assertEqual(self.runs(), ["1", "3"])

    def test_transport_attempt_reordering_does_not_create_history(self):
        first = receipt("available", at="1")
        first["transport"] = {"attempts": [
            {"ssh": {"state": "reachable"}, "screen-sharing": {"state": "timeout"}},
            {"ssh": {"state": "timeout"}, "screen-sharing": {"state": "reachable"}},
        ]}
        second = receipt("available", at="2")
        second["transport"] = {"attempts": list(reversed(first["transport"]["attempts"]))}
        self.publish(first, 1)
        self.publish(second, 2)
        self.assertEqual(self.runs(), ["1"])

    def test_gui_reason_change_is_retained(self):
        first = receipt("available", at="1")
        first["gui_session"].update(status="deferred", reason="first failure")
        second = receipt("available", at="2")
        second["gui_session"].update(status="deferred", reason="second failure")
        self.publish(first, 1)
        self.publish(second, 2)
        self.assertEqual(self.runs(), ["1", "2"])

    def test_malformed_current_attempts_count_as_change(self):
        with open(self.current, "w", encoding="utf-8") as f:
            f.write('{"admission": {"state": "available"}, "gui_session": {"gui_state": "unknown"}, "transport": {"attempts": null}}')
        self.publish(receipt("available"), 1)
        self.assertEqual(self.runs(), ["1"])

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
