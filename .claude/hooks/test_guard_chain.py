import importlib.util
import json
from pathlib import Path
import shutil
import tempfile
import unittest

s = importlib.util.spec_from_file_location('chain', Path(__file__).with_name('guard-pretool-chain.py'))
chain = importlib.util.module_from_spec(s)
s.loader.exec_module(chain)


class ChainTest(unittest.TestCase):
    def fixture(self, directory, legacy='#!/bin/sh\nexit 0\n'):
        folder = Path(directory)
        old = folder/'guard-pretool.original-20261016'
        old.write_text(legacy)
        old.chmod(0o700)
        shutil.copyfile(Path(__file__).with_name('maat-heavy-launch.py'), folder/'maat-heavy-launch.py')
        return folder

    def test_installed_chain_refuses_raw_detached_and_foreground_jobs(self):
        with tempfile.TemporaryDirectory() as d:
            folder = self.fixture(d)
            for command in ['nohup python3 probe.py &', 'python3 probe.py', 'go test ./...', './apollo-model']:
                self.assertEqual(chain.check(json.dumps({'tool_name':'Bash','tool_input':{'command':command}}).encode(), folder), 2)

    def test_existing_legacy_denial_remains_denial(self):
        with tempfile.TemporaryDirectory() as d:
            folder = self.fixture(d, '#!/bin/sh\nexit 2\n')
            self.assertEqual(chain.check(b'{"tool_name":"Read"}', folder), 2)

    def test_read_and_registration_remain_available(self):
        with tempfile.TemporaryDirectory() as d:
            folder = self.fixture(d)
            for command in ['git status', 'rg mlx source.go', 'sirsi thread register --agent own']:
                self.assertEqual(chain.check(json.dumps({'tool_name':'Bash','tool_input':{'command':command}}).encode(), folder), 0)

    def test_partial_install_refuses(self):
        with tempfile.TemporaryDirectory() as d:
            self.assertEqual(chain.check(b'{}', Path(d)), 2)


if __name__ == '__main__':
    unittest.main()
