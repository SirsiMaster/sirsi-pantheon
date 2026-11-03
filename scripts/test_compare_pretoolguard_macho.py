import importlib.util
import os
import json
import struct
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location('macho_compare', Path(__file__).with_name('compare-pretoolguard-macho.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def image(vmsize=0x1235):
    header = struct.pack('<8I', 0xfeedfacf, 0x0100000c, 0, 2, 1, 72, 0, 0)
    segment = struct.pack('<II16sQQQQiiII', 0x19, 72, b'__LINKEDIT',
                          0x100000000, vmsize, 104, 32, 1, 1, 0, 0)
    return header + segment + bytes(range(32))


class MachoIdentityTests(unittest.TestCase):
    def test_identical_and_exact_page_rounding_accept(self):
        self.assertEqual(module.compare(image(), image())['input_linkedit_vmsize'], 0x1235)
        self.assertEqual(module.compare(image(), image(0x4000))['output_linkedit_vmsize'], 0x4000)

    def test_unapproved_size_and_any_other_byte_refuse(self):
        candidates = [image(0x1236), image(0x8000), image(0x1000), image() + b'x']
        changed_payload = bytearray(image(0x4000)); changed_payload[-1] ^= 1
        changed_vmaddr = bytearray(image(0x4000)); changed_vmaddr[32 + 24] ^= 1
        candidates += [bytes(changed_payload), bytes(changed_vmaddr)]
        for candidate in candidates:
            with self.subTest(candidate=candidate[:40]):
                with self.assertRaises(ValueError):
                    module.compare(image(), candidate)
        with self.assertRaises(ValueError):
            module.compare(image(0x4000), image())

    def test_malformed_or_non_arm64_inputs_refuse_even_if_identical(self):
        candidates = []
        for offset, value in ((0, 0), (4, 0x01000007), (12, 6), (16, 0),
                              (20, 71), (32, 0x1d), (36, 8), (32 + 64, 1)):
            data = bytearray(image()); struct.pack_into('<I', data, offset, value); candidates.append(bytes(data))
        candidates += [image()[:60], image().replace(b'__LINKEDIT', b'__NOTLINK_', 1)]
        header = struct.pack('<8I', 0xfeedfacf, 0x0100000c, 0, 2, 2, 144, 0, 0)
        segment = struct.pack('<II16sQQQQiiII', 0x19, 72, b'__LINKEDIT',
                              0x100000000, 0x1235, 176, 32, 1, 1, 0, 0)
        candidates.append(header + segment + segment + bytes(range(32)))
        writable = bytearray(image()); struct.pack_into('<i', writable, 32 + 56, 3)
        executable = bytearray(image()); struct.pack_into('<i', executable, 32 + 60, 5)
        candidates += [bytes(writable), bytes(executable)]
        for candidate in candidates:
            with self.subTest(candidate=candidate[:40]):
                with self.assertRaises(ValueError):
                    module.compare(candidate, candidate)

    def test_rounding_into_another_segment_refuses(self):
        header = struct.pack('<8I', 0xfeedfacf, 0x0100000c, 0, 2, 2, 144, 0, 0)
        linkedit = struct.pack('<II16sQQQQiiII', 0x19, 72, b'__LINKEDIT',
                               0x100000000, 0x1235, 176, 32, 1, 1, 0, 0)
        following = struct.pack('<II16sQQQQiiII', 0x19, 72, b'__OTHER',
                                0x100002000, 0x1000, 0, 0, 1, 1, 0, 0)
        left = header + linkedit + following + bytes(range(32))
        right = bytearray(left); struct.pack_into('<Q', right, 32 + 32, 0x4000)
        with self.assertRaises(ValueError):
            module.compare(left, bytes(right))

    def test_regular_file_read_and_symlink_refusal(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'input'; path.write_bytes(image())
            self.assertEqual(module.read_file(str(path)), image())
            alias = Path(directory) / 'alias'; alias.symlink_to(path)
            with self.assertRaises(OSError):
                module.read_file(str(alias))

    def test_refusal_retains_exclusive_bounded_hash_report(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            left, right, report = root / 'left', root / 'right', root / 'normalization.json'
            left.write_bytes(image()); right.write_bytes(image(0x1236))
            command = [sys.executable, '-B', str(Path(module.__file__)), str(left), str(right), str(report)]
            result = subprocess.run(command, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            data = json.loads(report.read_text())
            self.assertEqual(data['status'], 'refused')
            self.assertEqual(len(data['input_sha256']), 64)
            self.assertEqual(len(data['output_sha256']), 64)
            first = report.read_bytes()
            self.assertNotEqual(subprocess.run(command, capture_output=True).returncode, 0)
            self.assertEqual(report.read_bytes(), first)


if __name__ == '__main__':
    unittest.main()
