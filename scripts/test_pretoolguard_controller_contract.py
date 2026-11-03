import subprocess
import tempfile
import os
import unittest
from pathlib import Path


class ControllerBridgeTests(unittest.TestCase):
    def test_dual_source_binding_and_no_publish_or_install(self):
        path = Path(__file__).with_name('build-release-pretoolguard-controller.sh')
        text = path.read_text()
        self.assertEqual(subprocess.run(['/bin/bash', '-n', str(path)], capture_output=True).returncode, 0)
        for term in ('source identity or cleanliness mismatch', 'controller identity or cleanliness mismatch',
                     '/usr/bin/cmp -s "$source_root/tools/signing/sirsi-sign"',
                     '/usr/bin/cmp -s "$source_root/scripts/verify-pretoolguard-entitlements.py"',
                     'controller-provenance.json', 'bash scripts/build-pretoolguard-artifact.sh',
                     'bash "$controller/scripts/sign-pretoolguard-artifact.sh"',
                     'PANTHEON_SIGN_CLIENT="$source_root/tools/signing/sirsi-sign" bash scripts/package-pretoolguard-artifact.sh',
                     'os.fsync(directory)', 'controller provenance readback mismatch', "open(target, 'xb')"):
            self.assertIn(term, text)
        self.assertLess(text.index('controller-provenance.json'), text.index('bash "$controller/scripts/sign-pretoolguard-artifact.sh"'))
        self.assertNotIn('gh release', text)
        self.assertNotIn('sudo', text)

    def test_packaging_invocation_uses_source_signer_not_controller_signer(self):
        text = Path(__file__).with_name('build-release-pretoolguard-controller.sh').read_text()
        line = next(line for line in text.splitlines() if line.startswith('PANTHEON_SIGN_CLIENT=') and 'package-pretoolguard-artifact.sh' in line)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory); (root / 'scripts').mkdir()
            stub = root / 'scripts/package-pretoolguard-artifact.sh'
            stub.write_text('#!/bin/bash\nprintf "%s" "$PANTHEON_SIGN_CLIENT"\n')
            env = dict(os.environ, source_root=str(root), PANTHEON_SIGN_CLIENT='/other/controller/sirsi-sign',
                       guard='g', guard_hash='h', guard_id='i', installer='j', installer_hash='k',
                       installer_id='l', attempt='m')
            result = subprocess.run(['/bin/bash', '-c', line], cwd=root, env=env, capture_output=True, text=True, check=True)
            self.assertEqual(result.stdout, str(root / 'tools/signing/sirsi-sign'))


if __name__ == '__main__':
    unittest.main()
