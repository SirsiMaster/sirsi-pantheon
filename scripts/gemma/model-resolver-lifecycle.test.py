#!/usr/bin/env python3
"""Whole legacy resolver with owned inert commands; no models/network/services.

Unlike the conf-block fixture, this runs the complete versioned shell program,
including downstream prefetch/probe flow. RESOLVER selects only the script under
test, enabling a production-only guard-omission negative control.
"""

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


MODEL = "mlx-community/gemma-4-31B-it-qat-4bit"
FALLBACK = "mlx-community/gemma-4-12B-it-8bit"
RESOLVER = Path(os.environ.get("RESOLVER", Path(__file__).with_name(
    "sirsi-gemma-model-resolver.sh"))).resolve(strict=True)

# Each executable is owned by the fixture, including the Python probe shim.
# Real Python runs only -c arithmetic/ranking; a probe's stdin is consumed but
# NEVER executed. No external endpoint or real process inventory is consulted.
DOUBLE = r'''
import json, os, sys
from pathlib import Path
name = Path(sys.argv[0]).name
root = Path(os.environ["FIXTURE_ROOT"])
args = sys.argv[1:]
def record(kind):
    with (root / "calls.jsonl").open("a") as out:
        out.write(json.dumps({"kind": kind, "args": args}) + "\n")
if name == "python3":
    if args and args[0] == "-c":
        os.execv(REAL_PYTHON, [REAL_PYTHON] + args)
    if args and args[0] == "-":
        sys.stdin.read()
        record("probe")
        print(30)
        sys.exit(0)
elif name == "sysctl" and args == ["-n", "hw.memsize"]:
    print(137438953472)
    sys.exit(0)
elif name == "ps":
    record("ps")
    sys.exit(0)
elif name == "curl":
    if args[-1] == "https://huggingface.co/api/models?author=mlx-community&search=gemma&sort=createdAt&limit=60":
        record("catalog")
        print(json.dumps([{"id": MODEL}]))
        sys.exit(0)
    if args[-1] == "http://127.0.0.1:11434/v1/models":
        record("probe-endpoint")
        sys.exit(0)
elif name == "hf":
    record("download")
    sys.exit(0)
record("unexpected-" + name)
sys.exit(99)
'''


class ResolverLifecycleTests(unittest.TestCase):
    def run_resolver(self, cached=(), conf=None, opt_in=None, downloader=True):
        with tempfile.TemporaryDirectory(prefix="gemma-resolver-lifecycle-") as tmp:
            root = Path(tmp)
            home, binaries = root / "home", root / "bin"
            home.mkdir(mode=0o700)
            binaries.mkdir(mode=0o700)
            (home / ".sirsi").mkdir(mode=0o700)
            if conf is not None:
                (home / ".sirsi/gemma-model.conf").write_text(conf)
            for model in cached:
                (home / ".cache/huggingface/hub" /
                 ("models--" + model.replace("/", "--"))).mkdir(parents=True)
            shim = ("#!" + sys.executable + "\nREAL_PYTHON = " +
                    repr(sys.executable) + "\nMODEL = " + repr(MODEL) + "\n" + DOUBLE)
            for name in ("python3", "sysctl", "ps", "curl", "pkill", "lsof", "sleep"):
                target = binaries / name
                target.write_text(shim)
                target.chmod(0o700)
            if downloader:
                target = home / ".venvs/mlx/bin/hf"
                target.parent.mkdir(parents=True)
                target.write_text(shim)
                target.chmod(0o700)
            # PATH contains only inert doubles plus these non-network utilities.
            for name in ("cat", "date", "mkdir", "dirname", "cut", "sed",
                         "tr", "grep", "tail", "seq"):
                utility = shutil.which(name)
                self.assertIsNotNone(utility, name)
                (binaries / name).symlink_to(utility)
            sirsi = home / ".local/bin/sirsi"
            sirsi.parent.mkdir(parents=True)
            sirsi.write_text(shim)
            sirsi.chmod(0o700)
            env = {"HOME": str(home), "PATH": str(binaries), "TMPDIR": str(root),
                   "FIXTURE_ROOT": str(root), "LC_ALL": "C"}
            if opt_in is not None:
                env["GEMMA_RESOLVER_PREFETCH"] = opt_in
            # Source the ENTIRE actual program; wait joins any fake background
            # downloader on normal return, so absence assertions cannot race it.
            # WEDGE exits the shell before any such child may be started.
            result = subprocess.run(
                ["/bin/bash", "-c", 'source "$1"; status=$?; wait; exit "$status"',
                 "resolver-fixture", str(RESOLVER)],
                env=env, cwd=root, capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)
            calls_path = root / "calls.jsonl"
            calls = ([json.loads(line) for line in calls_path.read_text().splitlines()]
                     if calls_path.exists() else [])
            self.assertFalse([x for x in calls if x["kind"].startswith("unexpected-")], calls)
            self.assertEqual(len([x for x in calls if x["kind"] == "catalog"]), 1, calls)
            return ((home / ".sirsi/gemma-model.conf").read_text(),
                    (home / ".sirsi/gemma-model-resolver.log").read_text(),
                    result.stdout, calls)

    def assert_wedge(self, conf=None, opt_in=None):
        actual, log, stdout, calls = self.run_resolver(conf=conf, opt_in=opt_in)
        expected = conf if conf is not None else FALLBACK + "\n"
        self.assertEqual(actual, expected)
        self.assertEqual(stdout, expected)
        self.assertIn("WEDGE:", log)
        self.assertFalse([x for x in calls if x["kind"] in
                          ("download", "probe-endpoint", "probe")], calls)

    def assert_cached(self, cached, serving, opt_in=None, downloader=True):
        actual, log, stdout, calls = self.run_resolver(
            cached=cached, opt_in=opt_in, downloader=downloader)
        self.assertEqual(actual, serving + "\n")
        self.assertEqual(stdout, serving + "\n")
        downloads = [x["args"] for x in calls if x["kind"] == "download"]
        self.assertEqual(downloads, [["download", serving]]
                         if opt_in == "1" and downloader else [])
        # Positive probe controls prove this is a downstream-flow fixture, not
        # an extraction that stopped before the original defect.
        self.assertEqual(len([x for x in calls if x["kind"] == "probe-endpoint"]), 1)
        self.assertEqual([x["args"] for x in calls if x["kind"] == "probe"],
                         [["-", serving, "11434"], ["-", serving, "11434"]])
        return log

    def test_wedge_absent_conf_default_never_downloads_or_probes(self):
        self.assert_wedge()

    def test_wedge_absent_conf_opt_in_still_never_downloads_or_probes(self):
        self.assert_wedge(opt_in="1")

    def test_wedge_preserves_existing_conf_despite_opt_in(self):
        self.assert_wedge(conf="prior-model-sentinel\n", opt_in="1")

    def test_cached_chosen_default_never_prefetches(self):
        self.assert_cached((MODEL,), MODEL)

    def test_cached_fallback_default_never_prefetches(self):
        self.assert_cached((FALLBACK,), FALLBACK)

    def test_cached_chosen_explicit_opt_in_prefetches_only_serving_model(self):
        self.assert_cached((MODEL,), MODEL, opt_in="1")

    def test_cached_fallback_explicit_opt_in_never_downloads_missing_choice(self):
        self.assert_cached((FALLBACK,), FALLBACK, opt_in="1")

    def test_nonliteral_opt_in_never_prefetches(self):
        for value in ("", "0", "true", "yes"):
            with self.subTest(value=value):
                self.assert_cached((MODEL,), MODEL, opt_in=value)

    def test_explicit_opt_in_missing_downloader_warns(self):
        log = self.assert_cached((MODEL,), MODEL, opt_in="1", downloader=False)
        self.assertIn("WARN prefetch skipped", log)


if __name__ == "__main__":
    unittest.main(verbosity=2)
