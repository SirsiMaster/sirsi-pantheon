"""Pure route fixtures plus real ssh -G parsing; no network or model calls."""
import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import Mock, patch

spec = importlib.util.spec_from_file_location("fabric_endpoints", Path(__file__).with_name("fabric-endpoints.py"))
policy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(policy)


class EndpointPolicy(unittest.TestCase):
    def test_all_six_directed_pairs_choose_thunderbolt_before_dns(self):
        for origin in policy.PEERS:
            for target in policy.PEERS:
                if origin == target:
                    continue
                with self.subTest(origin=origin, target=target):
                    dns = Mock(side_effect=AssertionError("DNS must wait until TB fails"))
                    route = policy.select_endpoint(origin, target, None, resolve=dns,
                                                   probe=lambda *_: True, relay_probe=lambda *_: True)
                    self.assertTrue(route[0].startswith("thunderbolt"))
                    self.assertEqual(route[2] is not None, {origin, target} == {"m1", "m5"})
                    dns.assert_not_called()

    def test_relay_keeps_two_distinct_pinned_hops(self):
        for origin, target in (("m1", "m5"), ("m5", "m1")):
            route = policy.select_endpoint(origin, target, None, resolve=lambda _: [],
                                           probe=lambda *_: True, relay_probe=lambda *_: True)
            self.assertIn(route[2], policy.THUNDERBOLT[(origin, "studio")])
            self.assertIn(route[1], policy.THUNDERBOLT[("studio", target)])
            args = policy.relay_ssh(route[2], None, route[1])
            self.assertIn("-oHostKeyAlias=sirsi-studio", args)
            self.assertIn("-oStrictHostKeyChecking=yes", args)
            self.assertIn("-oBatchMode=yes", args)
            self.assertIn("-W", args)
            self.assertNotIn("StrictHostKeyChecking=no", " ".join(args))

    def test_secondary_direct_tb_precedes_lan(self):
        result = policy.select_endpoint("m5", "studio", None, resolve=lambda _: [],
                                        probe=lambda a, _: a == "10.96.195.2")
        self.assertEqual(result, ("thunderbolt", "10.96.195.2", None))

    def test_lan_fallback_all_six_pairs_and_mixed_mdns_classification(self):
        answers = ["10.96.127.2", "169.254.2.3", "127.0.0.1", "100.87.95.88",
                   "8.8.8.8", "::1", "fe80::1", "192.168.1.208", "192.168.1.208"]
        for origin in policy.PEERS:
            for target in policy.PEERS:
                if origin == target:
                    continue
                route = policy.select_endpoint(origin, target, None, resolve=lambda _: answers,
                                               probe=lambda a, _: a == "192.168.1.208")
                self.assertEqual(route, ("lan", "192.168.1.208", None))
        self.assertFalse(policy.is_lan("10.96.178.2"))

    def test_tail_fallback_all_six_pairs(self):
        for origin in policy.PEERS:
            for target, peer in policy.PEERS.items():
                if origin == target:
                    continue
                result = policy.select_endpoint(origin, target, None,
                                                resolve=lambda _: ["192.168.1.208"],
                                                probe=lambda a, _: a == peer["tail"])
                self.assertEqual(result, ("tailscale", peer["tail"], None))

    def test_unknown_origin_does_not_invent_tb_link(self):
        rows = list(policy.candidates(None, "m1", lambda _: []))
        self.assertEqual(rows, [("tailscale", policy.PEERS["m1"]["tail"], None)])

    def test_total_outage_is_loud(self):
        with self.assertRaisesRegex(policy.PolicyError, "no reachable"):
            policy.select_endpoint("m5", "studio", None, resolve=lambda _: [], probe=lambda *_: False)

    def test_failed_relay_authentication_never_falls_back_to_lan(self):
        dns = Mock(side_effect=AssertionError("no fallback after identity rejection"))
        with patch.object(policy.subprocess, "run", return_value=Mock(returncode=255)):
            with self.assertRaisesRegex(policy.PolicyError, "no fallback"):
                policy.select_endpoint("m1", "m5", None, resolve=dns,
                                       probe=lambda *_: True, relay_probe=policy.relay_reachable)
        dns.assert_not_called()

    def test_relay_tcp_unavailable_can_fall_through(self):
        result = policy.select_endpoint("m1", "m5", None, resolve=lambda _: ["192.168.1.183"],
                                        probe=lambda *_: True, relay_probe=lambda *_: False)
        self.assertEqual(result, ("lan", "192.168.1.183", None))

    def test_proxy_is_used_for_probe_and_final_tunnel(self):
        proxy = "127.0.0.1:19001"
        with patch.object(policy.socket, "create_connection") as direct, \
             patch.object(policy.subprocess, "run", return_value=Mock(returncode=0)) as nc:
            self.assertTrue(policy.reachable("10.96.127.2", proxy))
            direct.assert_not_called()
            self.assertEqual(nc.call_args.args[0], policy.nc_args("10.96.127.2", proxy, probe=True))
        self.assertIn(proxy, policy.nc_args("10.96.127.2", proxy))
        self.assertIn(proxy, " ".join(policy.relay_ssh("10.96.178.2", proxy, "10.96.127.1")))

    def test_proxy_validation_and_no_bypass(self):
        for mode, env in (("direct", {"ALL_PROXY": "socks5://127.0.0.1:1234"}),
                          ("direct", {"HTTPS_PROXY": "https://example.invalid"}),
                          ("auto", {"HTTPS_PROXY": "https://example.invalid"}),
                          ("socks", {}), ("auto", {"ALL_PROXY": "socks5://192.168.1.1:1"}),
                          ("auto", {"ALL_PROXY": "socks5://127.0.0.1:0"}),
                          ("auto", {"ALL_PROXY": "socks5://127.0.0.1:65536"})):
            with self.subTest(mode=mode, env=env), self.assertRaises(policy.PolicyError):
                policy.runtime_proxy(mode, env)
        self.assertEqual(policy.runtime_proxy("socks", {"ALL_PROXY": "socks5h://127.0.0.1:1234"}), "127.0.0.1:1234")
        self.assertIsNone(policy.runtime_proxy("auto", {}))

    def test_main_never_retries_after_selected_tunnel_failure(self):
        with patch.dict(os.environ, {}, clear=True), \
             patch.object(policy, "origin_host", return_value="m5"), \
             patch.object(policy, "select_endpoint", return_value=("thunderbolt", "10.96.127.2", None)) as select, \
             patch.object(policy.os, "execv", side_effect=OSError("fixture failure")) as execute:
            self.assertEqual(policy.main(["connect", "studio"]), 78)
            self.assertEqual(select.call_count, 1)
            self.assertEqual(execute.call_count, 1)

    def test_render_is_pure_and_real_ssh_config_parses_all_identities(self):
        helper = str(Path(policy.__file__).resolve())
        with patch.object(policy, "reachable", side_effect=AssertionError("render must not probe")):
            config = policy.render_config(helper, "/usr/bin/python3")
        with tempfile.TemporaryDirectory(prefix="fabric-config-fixture-") as directory:
            path = Path(directory) / "config"
            path.write_text(config)
            for target, peer in policy.PEERS.items():
                result = subprocess.run(["/usr/bin/ssh", "-G", "-F", str(path), "sirsi-" + target],
                                        text=True, capture_output=True, check=False, timeout=5)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("hostkeyalias sirsi-" + target, result.stdout)
                self.assertIn("user " + peer["user"], result.stdout)
                self.assertIn("stricthostkeychecking true", result.stdout)
                self.assertIn(helper, result.stdout)
                self.assertIn("--transport auto", result.stdout)


if __name__ == "__main__":
    unittest.main()
