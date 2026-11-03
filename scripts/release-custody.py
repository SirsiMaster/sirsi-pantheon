#!/usr/bin/env python3
"""Retain/restore exact release inputs for the same GitHub workflow run.

This does not sign, publish, or alter a release. GitHub run-scoped artifact
custody is an explicit trust boundary; the publisher still verifies signatures,
the peeled tag, all asset bytes and the current release state before mutation.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import posixpath
import shutil
import stat
import subprocess
import sys
import unicodedata
import zipfile


def sha(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for data in iter(lambda: f.read(1024 * 1024), b''):
            h.update(data)
    return h.hexdigest()


def copy_exclusive(source, target):
    with source.open('rb') as src, target.open('xb') as dst:
        shutil.copyfileobj(src, dst, 1024 * 1024)
        dst.flush()
        os.fsync(dst.fileno())
    if sha(source) != sha(target):
        raise RuntimeError('custody copy differs')


def inspect_app_archive(path):
    # The archive is not executable authority. Bound paths and symlinks before
    # extraction; a later codesign verification still decides app acceptance.
    seen, canonical_seen, total, links = set(), set(), 0, {}
    def canonical(value):
        return unicodedata.normalize('NFD', value).casefold()
    with zipfile.ZipFile(path) as archive:
        items = archive.infolist()
        if not items or len(items) > 100000:
            raise RuntimeError('invalid app archive member count')
        for item in items:
            name = item.filename.rstrip('/')
            parts = name.split('/')
            if (not name or name.startswith('/') or '\\' in name or
                    any(p in ('', '.', '..') for p in parts) or
                    parts[0] not in ('Pantheon.app', '__MACOSX') or name in seen or
                    canonical(name) in canonical_seen):
                raise RuntimeError('unsafe or duplicate app archive path')
            seen.add(name)
            canonical_seen.add(canonical(name))
            total += item.file_size
            if total > 4 * 1024 ** 3:
                raise RuntimeError('app archive exceeds extraction bound')
            mode = item.external_attr >> 16
            kind = stat.S_IFMT(mode)
            if kind == stat.S_IFLNK:
                if parts[0] != 'Pantheon.app' or item.file_size > 4096:
                    raise RuntimeError('invalid app archive symlink')
                target = archive.read(item).decode('utf-8')
                resolved = posixpath.normpath(posixpath.join(posixpath.dirname(name), target))
                if target.startswith('/') or not resolved.startswith('Pantheon.app/'):
                    raise RuntimeError('app archive symlink escapes app')
                links[canonical(name)] = (posixpath.dirname(name), target)
            elif kind not in (0, stat.S_IFREG, stat.S_IFDIR):
                raise RuntimeError('unsupported app archive entry')
    if 'Pantheon.app' not in seen and not any(name.startswith('Pantheon.app/') for name in seen):
        raise RuntimeError('app archive has no app')
    for name in seen:
        parents = name.split('/')[:-1]
        if any(canonical('/'.join(parents[:i])) in links for i in range(1, len(parents) + 1)):
            raise RuntimeError('app archive entry traverses symlink')
    for parent, target in links.values():
        stack = parent.split('/')
        for part in target.split('/'):
            if part in ('', '.'):
                continue
            if part == '..':
                if len(stack) <= 1:
                    raise RuntimeError('app archive symlink escapes app')
                stack.pop()
            else:
                stack.append(part)
                if canonical('/'.join(stack)) in links:
                    raise RuntimeError('app archive symlink chain refused')


def main():
    if len(sys.argv) != 6:
        raise SystemExit('usage: release-custody.py create|restore <directory> <version> <commit> <manifest-path>')
    _, mode, directory, version, commit, manifest = sys.argv
    if (mode not in ('create', 'restore', 'check-receipt') or not re.fullmatch(r'[0-9a-f]{40}', commit) or
            not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?', version)):
        raise SystemExit('invalid custody identity')
    custody, root = Path(directory), Path.cwd()
    if mode == 'check-receipt':
        run_id, attempt = os.environ.get('GITHUB_RUN_ID', ''), os.environ.get('GITHUB_RUN_ATTEMPT', '')
        if not run_id.isdigit() or not attempt.isdigit() or int(attempt) < 2:
            raise SystemExit('invalid prior receipt identity')
        filename = f'pantheon-publication-{run_id}-{int(attempt) - 1}.json'
        receipts = list(custody.rglob(filename))
        if len(receipts) != 1 or not receipts[0].is_file() or receipts[0].is_symlink():
            raise SystemExit('previous publication receipt missing or ambiguous; reconcile mount custody')
        previous = json.loads(receipts[0].read_text())
        if (previous.get('commit') != commit or previous.get('tag') != 'v' + version or
                not previous.get('guard_attachment_metadata')):
            raise SystemExit('previous guard mount outcome not proven; reconcile before retry')
        return
    names = [f'SirsiPantheon-{version}-arm64.dmg', f'SirsiPantheon-{version}-arm64.pkg',
             f'SirsiPantheon-{version}-arm64-pretoolguard.dmg']
    entries = names + ['payload-manifest.json', 'Pantheon.app.zip']
    if mode == 'create':
        if not custody.is_dir() or any(custody.iterdir()):
            raise SystemExit('custody directory must be new and empty')
        for name in names:
            copy_exclusive(root / 'bin' / name, custody / name)
        copy_exclusive(Path(manifest), custody / 'payload-manifest.json')
        subprocess.run(['/usr/bin/ditto', '-c', '-k', '--sequesterRsrc', '--keepParent',
                        str(root / 'Pantheon.app'), str(custody / 'Pantheon.app.zip')], check=True, timeout=180)
        record = {'commit': commit, 'version': version, 'sha256': {name: sha(custody / name) for name in entries}}
        with (custody / 'custody.json').open('x') as f:
            json.dump(record, f, sort_keys=True); f.write('\n'); f.flush(); os.fsync(f.fileno())
    else:
        record = json.loads((custody / 'custody.json').read_text())
        if (record.get('commit') != commit or record.get('version') != version or
                set(record.get('sha256', {})) != set(entries)):
            raise SystemExit('custody identity mismatch')
        for name in entries:
            if sha(custody / name) != record['sha256'][name]:
                raise SystemExit('custody digest mismatch: ' + name)
        inspect_app_archive(custody / 'Pantheon.app.zip')
        if (root / 'Pantheon.app').exists() or (root / 'Pantheon.app').is_symlink():
            raise SystemExit('existing app: refusing overwrite')
        (root / 'bin').mkdir(exist_ok=True)
        for name in names:
            copy_exclusive(custody / name, root / 'bin' / name)
        subprocess.run(['/usr/bin/ditto', '-x', '-k', str(custody / 'Pantheon.app.zip'), str(root)],
                       check=True, timeout=180)


if __name__ == '__main__':
    main()
