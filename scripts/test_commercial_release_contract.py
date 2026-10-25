"""Inert source-verifier fixtures; publisher/GitHub/Apple commands never run."""
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent


class PublisherSourceContract(unittest.TestCase):
    def check_variant(self, old=None, replacement=None):
        with tempfile.TemporaryDirectory(prefix='pantheon-commercial-source-contract-') as temp:
            root = Path(temp)
            for name in ('.github', 'Makefile', 'contracts', 'cmd', 'internal', 'macapp'):
                (root/name).symlink_to(ROOT/name, target_is_directory=(ROOT/name).is_dir())
            scripts = root/'scripts'
            scripts.mkdir()
            for source in (ROOT/'scripts').iterdir():
                if source.name not in ('verify-commercial-release-contract.sh', 'publish-release-assets.py'):
                    (scripts/source.name).symlink_to(source, target_is_directory=source.is_dir())
            shutil.copyfile(ROOT/'scripts/verify-commercial-release-contract.sh', scripts/'verify-commercial-release-contract.sh')
            publisher = (ROOT/'scripts/publish-release-assets.py').read_text()
            if old is not None:
                self.assertEqual(publisher.count(old), 1, old)
                publisher = publisher.replace(old, replacement, 1)
            (scripts/'publish-release-assets.py').write_text(publisher)
            return subprocess.run(['/bin/bash', str(scripts/'verify-commercial-release-contract.sh')], text=True, capture_output=True, timeout=20)

    def test_current_publisher_passes_actual_cumulative_contract(self):
        result = self.check_variant()
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertIn('commercial release contract: pass', result.stdout)

    def test_each_creation_identity_requirement_is_enforced(self):
        mutations = [
            ('"--method", "POST"', '"--method", "GET"'),
            ('"-F", "draft=true"', '"-F", "draft=false"'),
            ('"--jq", "{id,tag_name,draft}"', '"--jq", "{id}"'),
            ('"tag_name=" + tag', '"tag_name=unbound"'),
            ('"target_commitish=" + expected_commit', '"target_commitish=main"'),
            ('selected_id = identity["id"]', 'selected_id = None'),
            ('identity.get("draft") is not True', 'identity.get("draft") is False'),
            ('record.update(release_id=selected_id, creation_outcome="draft_identity_received")', 'record.update(creation_outcome="draft_identity_received")'),
        ]
        for old, replacement in mutations:
            with self.subTest(requirement=old):
                result = self.check_variant(old, replacement)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('draft publication/readback contract missing:', result.stderr)


if __name__ == '__main__':
    unittest.main(verbosity=2)
