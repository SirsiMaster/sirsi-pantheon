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
    def setUp(self):
        self.monitor_patch = patch.object(policy, "live_links", return_value=None)
        self.monitor_patch.start()
        self.addCleanup(self.monitor_patch.stop)
        self.route_patch = patch.object(policy, "route_is_thunderbolt", return_value=False)
        self.route_patch.start()
        self.addCleanup(self.route_patch.stop)

    def test_live_link_precedes_static_and_lan(self):
        with patch.object(policy, "live_links", return_value=["10.96.109.2"]), \
             patch.object(policy, "route_is_thunderbolt", return_value=True):
            rows = list(policy.candidates("m5", "studio", lambda _: ["192.168.1.208"]))
        self.assertEqual(rows[0], ("thunderbolt", "10.96.109.2", None))
        self.assertNotIn(("thunderbolt", "10.96.127.2", None), rows)

    def test_empty_live_view_preserves_relay_before_lan(self):
        with patch.object(policy, "live_links", return_value=[]):
            rows = list(policy.candidates("m1", "m5", lambda _: ["192.168.1.183"]))
        self.assertEqual(rows[0][0], "thunderbolt-relay")
        self.assertLess(next(i for i, row in enumerate(rows) if row[0] == "thunderbolt-relay"),
                        next(i for i, row in enumerate(rows) if row[0] == "lan"))

    def test_unproved_live_route_is_not_labeled_thunderbolt(self):
        with patch.object(policy, "live_links", return_value=["10.96.109.2"]), \
             patch.object(policy, "route_is_thunderbolt", return_value=False):
            rows = list(policy.candidates("m5", "studio", lambda _: ["192.168.1.208"]))
        self.assertEqual(rows[0], ("lan", "192.168.1.208", None))
        self.assertNotIn(("thunderbolt", "10.96.109.2", None), rows)

    def test_link_metadata_failures_never_supply_links(self):
        self.monitor_patch.stop()
        for payload in (None, "not-json", '{"local":"a","machines":{}}',
                        '{"local":"a","machines":{"a":{"profile":{"links":null}}}}'):
            with self.subTest(payload=payload), patch.object(policy, "metadata_output", return_value=payload):
                self.assertIsNone(policy.live_links("m1", "m5"))

    def test_monitor_unavailable_mdns_thunderbolt_is_route_classified(self):
        with patch.object(policy, "route_is_thunderbolt", side_effect=lambda a: a == "10.96.109.2"):
            rows = list(policy.candidates("m5", "studio", lambda _: ["10.96.109.2", "10.96.141.2", "192.168.1.208"]))
        self.assertLess(rows.index(("thunderbolt", "10.96.109.2", None)), rows.index(("lan", "192.168.1.208", None)))
        self.assertNotIn(("thunderbolt", "10.96.141.2", None), rows)

    def test_direct_mdns_precedes_relay(self):
        with patch.object(policy, "route_is_thunderbolt", return_value=True):
            rows = list(policy.candidates("m1", "m5", lambda _: ["10.96.109.2"]))
        self.assertEqual(rows[0], ("thunderbolt", "10.96.109.2", None))
        self.assertEqual(rows[1][0], "thunderbolt-relay")

    def test_discovery_respects_guard_and_caps_answers(self):
        guard = Mock(side_effect=[None] * 5 + [ValueError("expired")])
        with patch.object(policy, "route_is_thunderbolt", return_value=False) as classify:
            with self.assertRaisesRegex(ValueError, "expired"):
                list(policy.candidates("m1", "m5", lambda _: ["10.96.1.2", "10.96.1.3"], guard))
        classify.assert_called_once_with("10.96.1.2")
        rows = list(policy.candidates("m1", "m5", lambda _: ["10.96.1.2"] * 33))
        self.assertEqual(rows[0][0], "thunderbolt-relay")
        self.assertEqual(rows[-1][0], "tailscale")
        rows = list(policy.candidates("m1", "m5", lambda _: ["10.96.1.%d" % n for n in range(1, 6)]))
        self.assertEqual(rows[0][0], "thunderbolt-relay")

    def test_wrong_origin_or_malformed_peer_metadata_is_unavailable(self):
        import json
        self.monitor_patch.stop()
        for local, ip in (("m1", "10.96.1.2"), ("m5", "bad-address")):
            payload = json.dumps({"local": local, "machines": {local: {"profile": {"links": [
                {"peer_host": "M5-Studio", "peer_ip": ip}]}}}})
            with patch.object(policy, "metadata_output", return_value=payload):
                self.assertIsNone(policy.live_links("m5", "studio"))
        with patch.object(policy, "metadata_output", return_value="[" * 2000 + "]" * 2000):
            self.assertIsNone(policy.live_links("m5", "studio"))

    def test_local_link_metadata_is_bounded_and_peer_scoped(self):
        import json
        response = json.dumps({"local": "m5", "description": "Température 𓂀", "machines": {"m5": {"profile": {"links": [
            {"peer_host": "M5-Studio", "peer_ip": "10.96.109.2"},
            {"peer_host": "M1", "peer_ip": "10.96.178.2"},
            {"peer_host": "M5-Studio", "peer_ip": "192.168.1.208"}]}}}}, ensure_ascii=False)
        # Exercise the function itself, not the default setUp stub.
        self.monitor_patch.stop()
        with patch.object(policy, "metadata_output", return_value=response) as read:
            self.assertEqual(policy.live_links("m5", "studio"), ["10.96.109.2"])
        argv, limit = read.call_args.args
        self.assertEqual(argv[-1], "http://127.0.0.1:8099/api?meta=1")
        self.assertEqual(argv[1], "--disable")  # curl reads no default config or extra URLs
        self.assertEqual(limit, 262144)
        self.assertEqual(read.call_args.kwargs, {"encoding": "utf-8"})
        self.assertIn("--noproxy", argv)

    def test_metadata_output_utf8_is_strict_and_remains_bounded(self):
        raw = '{"description":"Température 𓂀"}'.encode('utf-8')
        command = ['/usr/bin/python3', '-I', '-c', 'import sys;sys.stdout.buffer.write(' + repr(raw) + ')']
        self.assertIsNone(policy.metadata_output(command, len(raw)))
        self.assertEqual(policy.metadata_output(command, len(raw), encoding='utf-8'), raw.decode('utf-8'))
        self.assertIsNone(policy.metadata_output(command, len(raw) - 1, encoding='utf-8'))
        invalid = ['/usr/bin/python3', '-I', '-c', "import sys;sys.stdout.buffer.write(b'\\xff')"]
        self.assertIsNone(policy.metadata_output(invalid, 1, encoding='utf-8'))

    def test_curl_disable_ignores_default_config_without_network(self):
        with tempfile.TemporaryDirectory(prefix="fabric-curl-config-") as root:
            directory = Path(root)
            (directory / '.curlrc').write_text('write-out = "CONFIG_WAS_READ"\n')
            payload = directory / 'payload.json'
            payload.write_text('{"fixture":true}', encoding='utf-8')
            env = {'HOME': root, 'PATH': '/usr/bin:/bin', 'LC_ALL': 'C'}
            base = ['/usr/bin/curl', '--silent', '--show-error', '--fail', payload.as_uri()]
            control = subprocess.run(base, env=env, capture_output=True, text=True, timeout=5, check=True)
            protected = subprocess.run([base[0], '--disable', *base[1:]], env=env,
                                       capture_output=True, text=True, timeout=5, check=True)
            self.assertIn('CONFIG_WAS_READ', control.stdout)
            self.assertEqual(protected.stdout, '{"fixture":true}')

    def test_live_links_decodes_actual_utf8_output_before_selecting_peer(self):
        import json
        raw = json.dumps({"local": "M5", "description": "Température 𓂀", "machines": {"M5": {"profile": {"links": [
            {"peer_host": "m5-studio", "peer_ip": "10.96.141.2"}]}}}}, ensure_ascii=False).encode('utf-8')
        native = policy.metadata_output
        def fixture_output(_argv, limit, encoding='ascii'):
            return native(['/usr/bin/python3', '-I', '-c', 'import sys;sys.stdout.buffer.write(' + repr(raw) + ')'],
                          limit, encoding=encoding)
        self.monitor_patch.stop()
        with patch.object(policy, "metadata_output", side_effect=fixture_output):
            self.assertEqual(policy.live_links("m5", "studio"), ["10.96.141.2"])

    def test_thunderbolt_route_parser_rejects_wifi_and_ambiguous_routes(self):
        ports = "Hardware Port: Thunderbolt 1\nDevice: en1\n\nHardware Port: Thunderbolt Bridge\nDevice: bridge0\n\nHardware Port: Wi-Fi\nDevice: en0\n"
        self.route_patch.stop()
        for route, expected in ((" interface: en1\n", True), (" interface: bridge0\n", True), (" interface: en0\n", False),
                                (" interface: en1\n interface: en0\n", False), ("", False)):
            with patch.object(policy, "metadata_output", side_effect=[ports, route]):
                self.assertEqual(policy.route_is_thunderbolt("10.96.109.2"), expected)

    def test_all_six_directed_pairs_choose_thunderbolt_before_dns(self):
        for origin in policy.PEERS:
            for target in policy.PEERS:
                if origin == target:
                    continue
                with self.subTest(origin=origin, target=target):
                    dns = Mock(return_value=[])
                    route = policy.select_endpoint(origin, target, None, resolve=dns,
                                                   probe=lambda *_: True, relay_probe=lambda *_: True)
                    self.assertTrue(route[0].startswith("thunderbolt"))
                    self.assertEqual(route[2] is not None, {origin, target} == {"m1", "m5"})
                    if {origin, target} != {"m1", "m5"}:
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
            for option in ("-oGlobalKnownHostsFile=/dev/null", "-oKnownHostsCommand=none", "-oVerifyHostKeyDNS=no", "-oUpdateHostKeys=no"):
                self.assertIn(option, args)
            self.assertIn("-W", args)
            self.assertNotIn("StrictHostKeyChecking=no", " ".join(args))

    def test_relay_effective_ssh_config_restricts_host_key_sources(self):
        args = policy.relay_ssh("10.96.178.2", None, "10.96.127.1", probe=True)
        with tempfile.TemporaryDirectory() as root:
            config = Path(root) / "config"
            config.write_text("Host *\n  KnownHostsCommand /usr/bin/false\n  GlobalKnownHostsFile /tmp/foreign-known-hosts\n")
            result = subprocess.run([args[0], "-G", "-F", str(config), *args[1:]], text=True, capture_output=True, timeout=5)
        self.assertEqual(result.returncode, 0, result.stderr)
        effective = result.stdout.lower()
        for item in ("stricthostkeychecking true", "globalknownhostsfile /dev/null",
                     "verifyhostkeydns false", "updatehostkeys false"):
            self.assertIn(item, effective)
        self.assertFalse(any(line.startswith("knownhostscommand ") and line != "knownhostscommand none"
                             for line in effective.splitlines()))

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
        dns = Mock(return_value=[])
        with patch.object(policy.subprocess, "run", return_value=Mock(returncode=255)):
            with self.assertRaisesRegex(policy.PolicyError, "no fallback"):
                policy.select_endpoint("m1", "m5", None, resolve=dns,
                                       probe=lambda *_: True, relay_probe=policy.relay_reachable)
        dns.assert_called_once()

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

    def test_native_connect_deadline_is_not_just_idle_timeout(self):
        probe = policy.nc_args("10.96.127.2", probe=True)
        self.assertEqual(probe[probe.index("-G") + 1], "2")
        stream = policy.nc_args("10.96.127.2")
        self.assertEqual(stream[stream.index("-G") + 1], "8")
        remote = policy.relay_ssh("10.96.178.2", None, "10.96.127.1", probe=True)
        command = remote[remote.index("/usr/bin/nc"):]
        self.assertEqual(command[command.index("-G") + 1], "2")

    def test_proxy_validation_and_no_bypass(self):
        for mode, env in (("direct", {"ALL_PROXY": "socks5://127.0.0.1:1234"}),
                          ("direct", {"HTTPS_PROXY": "https://example.invalid"}),
                          ("auto", {"HTTPS_PROXY": "https://example.invalid"}),
                          ("socks", {}), ("auto", {"ALL_PROXY": "socks5://192.168.1.1:1"}),
                          ("auto", {"ALL_PROXY": "socks5://127.0.0.1:0"}),
                          ("auto", {"ALL_PROXY": "socks5://127.0.0.1:65536"}),
                          ("auto", {"ALL_PROXY": "socks5://127.0.0.1:1234", "all_proxy": "socks5://127.0.0.1:1235"}),
                          ("socks", {"ALL_PROXY": "socks5://127.0.0.1:1234", "HTTPS_PROXY": "http://example.invalid"})):
            with self.subTest(mode=mode, env=env), self.assertRaises(policy.PolicyError):
                policy.runtime_proxy(mode, env)
        self.assertEqual(policy.runtime_proxy("socks", {"ALL_PROXY": "socks5h://127.0.0.1:1234"}), "127.0.0.1:1234")
        self.assertEqual(policy.runtime_proxy("auto", {"ALL_PROXY": "socks5://127.0.0.1:1234", "https_proxy": "socks5://127.0.0.1:1234"}), "127.0.0.1:1234")
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
            path.write_text(config + "  KnownHostsCommand /usr/bin/false\n  GlobalKnownHostsFile /tmp/foreign-known-hosts\n")
            for target, peer in policy.PEERS.items():
                result = subprocess.run(["/usr/bin/ssh", "-G", "-F", str(path), "sirsi-" + target],
                                        text=True, capture_output=True, check=False, timeout=5)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("hostkeyalias sirsi-" + target, result.stdout)
                self.assertIn("user " + peer["user"], result.stdout)
                self.assertIn("stricthostkeychecking true", result.stdout)
                self.assertIn("globalknownhostsfile /dev/null", result.stdout)
                self.assertIn("verifyhostkeydns false", result.stdout)
                self.assertFalse(any(line.startswith("knownhostscommand ") and line != "knownhostscommand none"
                                     for line in result.stdout.splitlines()))
                self.assertIn(helper, result.stdout)
                self.assertIn("--transport auto", result.stdout)

    def test_earlier_ssh_host_block_can_override_generated_trust(self):
        config = policy.render_config(str(Path(policy.__file__).resolve()), "/usr/bin/python3")
        with tempfile.TemporaryDirectory(prefix="fabric-order-fixture-") as directory:
            path = Path(directory) / "config"
            path.write_text("Host *\n  GlobalKnownHostsFile /tmp/foreign-known-hosts\n" + config)
            result = subprocess.run(["/usr/bin/ssh", "-G", "-F", str(path), "sirsi-m1"],
                                    text=True, capture_output=True, check=False, timeout=5)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("globalknownhostsfile /tmp/foreign-known-hosts", result.stdout)


if __name__ == "__main__":
    unittest.main()
