#!/usr/bin/env python3
"""Exercise both real context builders with an inert compiler, never a deploy.

The compiler models Go's executable output under umask 077. Git cleanliness is
stubbed so these fixtures also cover an uncommitted repair; other Git reads are
real. This proves packaging modes and argv, not compilation or VCS stamping.
"""
import argparse
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tempfile
import unittest


SOURCE = Path(__file__).resolve().parents[2]


class ContextModes(unittest.TestCase):
    def context(self, script):
        temporary = tempfile.TemporaryDirectory(prefix="sirsi-context-modes-")
        self.addCleanup(temporary.cleanup)
        root = Path(temporary.name)
        root.chmod(0o700)
        context = root / "context"
        context.mkdir(mode=0o700)
        private = context / "private-sentinel"
        private.write_bytes(b"synthetic private fixture\n")
        private.chmod(0o600)
        commands = root / "bin"
        commands.mkdir(mode=0o700)
        compiler = commands / "go"
        compiler.write_text(
            "#!" + os.sys.executable + "\n"
            "import json, os, pathlib, sys\n"
            "args = sys.argv[1:]\n"
            "assert args[0] == 'build'\n"
            "assert (os.environ['GOOS'], os.environ['GOARCH'], "
            "os.environ['CGO_ENABLED']) == ('linux', 'amd64', '0')\n"
            "pathlib.Path(os.environ['MODE_GO_ARGS']).write_text(json.dumps(args))\n"
            "output = pathlib.Path(args[args.index('-o') + 1])\n"
            "output.write_bytes(b'#!/bin/sh\\nexit 0\\n')\n"
            "output.chmod(0o700)\n"
        )
        compiler.chmod(0o700)
        git = commands / "git"
        git.write_text(
            "#!/bin/bash\n"
            "if [ \"${1:-}\" = -C ] && [ \"${3:-}\" = status ]; then exit 0; fi\n"
            "exec \"$MODE_REAL_GIT\" \"$@\"\n"
        )
        git.chmod(0o700)
        cloud = commands / "gcloud"
        cloud.write_text("#!/bin/sh\necho 'unexpected cloud call' >&2\nexit 99\n")
        cloud.chmod(0o700)
        args = root / "go-args.json"
        env = dict(os.environ, PATH=str(commands) + os.pathsep + os.environ["PATH"],
                   MODE_REAL_GIT=shutil.which("git"), MODE_GO_ARGS=str(args))
        result = subprocess.run(
            ["/bin/bash", "-c", 'umask 077; exec /bin/bash "$1" --context-only "$2"',
             "context-mode-fixture", str(SOURCE / "scripts/router-service" / script), str(context)],
            cwd=SOURCE, env=env, capture_output=True, text=True, timeout=20,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return context, json.loads(args.read_text())

    def public_modes(self, script, public_files):
        context, _ = self.context(script)
        self.assertEqual(stat.S_IMODE(context.stat().st_mode), 0o700, "private context widened")
        self.assertEqual(stat.S_IMODE((context / "private-sentinel").stat().st_mode), 0o600,
                         "unrelated private file widened")
        with self.subTest(file="sirsi"):
            self.assertEqual(stat.S_IMODE((context / "sirsi").stat().st_mode), 0o755,
                             "distroless nonroot cannot execute /sirsi")
        for name in public_files:
            with self.subTest(file=name):
                self.assertEqual(stat.S_IMODE((context / name).stat().st_mode), 0o644,
                                 "distroless nonroot cannot read copied public asset")

    def test_stage_public_modes_under_private_umask(self):
        self.public_modes("stage.sh", ["recipient-registry.json"])

    def test_board_public_modes_under_private_umask(self):
        self.public_modes("board.sh", ["recipient-registry.json", "machines.json", "CHANGELOG.md"])

    def test_stage_requires_vcs_stamping(self):
        _, args = self.context("stage.sh")
        self.assertIn("-buildvcs=true", args)
        self.assertNotIn("-buildvcs=false", args)

    def test_board_requires_vcs_stamping(self):
        _, args = self.context("board.sh")
        self.assertIn("-buildvcs=true", args)
        self.assertNotIn("-buildvcs=false", args)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-root", type=Path, default=SOURCE)
    options, remaining = parser.parse_known_args()
    SOURCE = options.source_root.resolve()
    unittest.main(argv=[os.sys.argv[0], *remaining])
