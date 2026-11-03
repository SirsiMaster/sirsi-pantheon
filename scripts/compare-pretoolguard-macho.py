#!/usr/bin/env python3
"""Compare two signature-stripped thin arm64 executables, failing closed.

Only the observed 16-KiB rounding of __LINKEDIT.vmsize may differ. All other
bytes, including every code and data byte, must be identical. This is not a
general Mach-O semantic equivalence checker.
"""
import hashlib
import json
import os
import stat
import struct
import sys

LIMIT = 64 * 1024 * 1024
PAGE = 16384


def read_file(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or not 32 <= before.st_size <= LIMIT:
            raise ValueError('input must be a bounded regular file')
        chunks, remaining = [], before.st_size
        while remaining:
            data = os.read(fd, min(remaining, 1024 * 1024))
            if not data:
                raise ValueError('short read')
            chunks.append(data)
            remaining -= len(data)
        after = os.fstat(fd)
        if (before.st_size, before.st_mtime_ns, before.st_ctime_ns) != (after.st_size, after.st_mtime_ns, after.st_ctime_ns):
            raise ValueError('input changed while reading')
        return b''.join(chunks)
    finally:
        os.close(fd)


def linkedit(data):
    if len(data) < 32:
        raise ValueError('truncated header')
    magic, cpu, _, kind, count, size, _, _ = struct.unpack_from('<8I', data)
    if magic != 0xfeedfacf or cpu != 0x0100000c or kind != 2 or not 1 <= count <= 4096:
        raise ValueError('not a bounded thin arm64 executable')
    end = 32 + size
    if end > len(data):
        raise ValueError('truncated commands')
    offset, found, ranges = 32, [], []
    for _ in range(count):
        if offset + 8 > end:
            raise ValueError('truncated command')
        command, command_size = struct.unpack_from('<II', data, offset)
        if command_size < 8 or command_size % 8 or offset + command_size > end:
            raise ValueError('invalid command size')
        if command == 0x1d:
            raise ValueError('signature command remains')
        if command == 0x19:
            if command_size < 72:
                raise ValueError('truncated segment')
            sections = struct.unpack_from('<I', data, offset + 64)[0]
            if command_size != 72 + sections * 80:
                raise ValueError('invalid segment sections')
            name = data[offset + 8:offset + 24]
            vmaddr, vmsize, fileoff, filesize = struct.unpack_from('<QQQQ', data, offset + 24)
            if vmaddr + vmsize >= 1 << 64:
                raise ValueError('segment virtual range overflows')
            if vmsize:
                ranges.append((vmaddr, vmaddr + vmsize, name))
            if name == b'__LINKEDIT\0\0\0\0\0\0':
                maxprot, initprot = struct.unpack_from('<ii', data, offset + 56)
                if (sections != 0 or maxprot != 1 or initprot != 1 or not filesize or filesize > vmsize or
                        fileoff < end or fileoff + filesize != len(data)):
                    raise ValueError('invalid linkedit bounds')
                found.append((offset + 32, vmsize))
        offset += command_size
    if offset != end or len(found) != 1:
        raise ValueError('linkedit command not unique')
    ranges.sort(key=lambda entry: entry[0])
    if any(current[0] < previous[1] for previous, current in zip(ranges, ranges[1:])):
        raise ValueError('overlapping segment virtual ranges')
    return found[0]


def compare(left, right):
    left_offset, left_size = linkedit(left)
    right_offset, right_size = linkedit(right)
    if left_offset != right_offset or len(left) != len(right):
        raise ValueError('layout differs')
    if left_size != right_size:
        if left_size > (1 << 64) - PAGE or right_size != ((left_size + PAGE - 1) // PAGE) * PAGE:
            raise ValueError('linkedit size is not the allowed page rounding')
    if left[:left_offset] != right[:right_offset] or left[left_offset + 8:] != right[right_offset + 8:]:
        raise ValueError('non-linkedit-size bytes differ')
    return {'input_linkedit_vmsize': left_size, 'output_linkedit_vmsize': right_size,
            'vmsize_offset': left_offset, 'bytes': len(left)}


def main():
    if len(sys.argv) != 4:
        raise SystemExit('usage: compare-pretoolguard-macho.py <input> <output> <new-report>')
    report = {'status': 'incomplete'}
    with open(sys.argv[3], 'x') as out:
        try:
            left, right = read_file(sys.argv[1]), read_file(sys.argv[2])
            report.update(input_sha256=hashlib.sha256(left).hexdigest(),
                          output_sha256=hashlib.sha256(right).hexdigest())
            report.update(compare(left, right), status='equivalent_bounded')
        except Exception as error:
            report.update(status='refused', error=str(error))
            raise
        finally:
            json.dump(report, out, sort_keys=True)
            out.write('\n'); out.flush(); os.fsync(out.fileno())


if __name__ == '__main__':
    main()
