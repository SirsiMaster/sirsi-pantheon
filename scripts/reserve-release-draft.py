#!/usr/bin/env python3
"""Reserve one source-bound private draft for the existing sole publisher.

No signing or asset upload occurs here. An uncertain create is never retried.
"""
import argparse
import json
import os
import re
import subprocess
import sys

REPO = 'SirsiMaster/sirsi-pantheon'


def execute(argv):
    return subprocess.run(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=60, check=False)


def reserve(version, commit, record, run=execute):
    if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?', version) or not re.fullmatch(r'[0-9a-f]{40}', commit):
        raise ValueError('exact version and commit required')
    tag = 'v' + version
    record.update(tag=tag, commit=commit, status='checking', creation_attempted=False)

    def checked(argv):
        result = run(argv)
        if result.returncode or len(result.stdout) + len(result.stderr) > 1024 * 1024:
            raise RuntimeError('reservation read or create outcome unknown')
        return result.stdout

    def check_tag():
        output = checked(['git', 'ls-remote', '--exit-code', 'origin', 'refs/tags/' + tag, 'refs/tags/' + tag + '^{}'])
        refs = dict(line.split('\t', 1)[::-1] for line in output.decode().splitlines())
        if refs.get('refs/tags/' + tag + '^{}', refs.get('refs/tags/' + tag)) != commit:
            raise RuntimeError('remote tag does not peel to expected commit')

    check_tag()
    # Discovery is complete only after a bounded final short page. Private
    # drafts need the authenticated list, not the public tag lookup alone.
    seen = set()
    for page in range(1, 11):
        endpoint = f'repos/{REPO}/releases?per_page=100&page={page}'
        data = json.loads(checked(['gh', 'api', '--hostname', 'github.com', '--jq',
                                   'map({id,tag_name,draft})', endpoint]))
        if not isinstance(data, list) or len(data) > 100:
            raise RuntimeError('invalid release discovery')
        for item in data:
            if (not isinstance(item, dict) or type(item.get('id')) is not int or item['id'] <= 0 or
                    item['id'] in seen or not isinstance(item.get('tag_name'), str) or
                    type(item.get('draft')) is not bool):
                raise RuntimeError('ambiguous release discovery')
            seen.add(item['id'])
            if item['tag_name'] == tag:
                raise RuntimeError('release already exists; reconcile before signing')
        if len(data) < 100:
            break
    else:
        raise RuntimeError('release discovery incomplete; reconcile before signing')
    check_tag()
    record.update(creation_attempted=True, status='create_outcome_unknown')
    response = checked(['gh', 'api', '--hostname', 'github.com', '--include', '--method', 'POST',
                        '--jq', '{id,tag_name,draft}', '-f', 'tag_name=' + tag,
                        '-f', 'target_commitish=' + commit, '-f', 'name=Sirsi Pantheon ' + version,
                        '-F', 'draft=true', '-F', 'generate_release_notes=true',
                        'repos/' + REPO + '/releases'])
    headers, sep, body = response.replace(b'\r\n', b'\n').partition(b'\n\n')
    if not sep or not re.match(rb'HTTP/\S+ 201(?: |\n|$)', headers):
        raise RuntimeError('draft creation outcome unknown; do not recreate')
    value = json.loads(body)
    if (not isinstance(value, dict) or set(value) != {'id', 'tag_name', 'draft'} or
            type(value.get('id')) is not int or value['id'] <= 0 or
            value.get('tag_name') != tag or value.get('draft') is not True):
        raise RuntimeError('draft creation identity invalid; do not recreate')
    record['release_id'] = value['id']
    # The response remains the sole selected identity; the second bounded
    # listing only detects ambiguity and never chooses a replacement draft.
    same_tag, seen = [], set()
    for page in range(1, 11):
        endpoint = f'repos/{REPO}/releases?per_page=100&page={page}'
        data = json.loads(checked(['gh', 'api', '--hostname', 'github.com', '--jq',
                                   'map({id,tag_name,draft})', endpoint]))
        if not isinstance(data, list) or len(data) > 100:
            raise RuntimeError('post-create release discovery unknown')
        for item in data:
            if (not isinstance(item, dict) or type(item.get('id')) is not int or item['id'] <= 0 or
                    item['id'] in seen or not isinstance(item.get('tag_name'), str) or
                    type(item.get('draft')) is not bool):
                raise RuntimeError('ambiguous post-create release discovery')
            seen.add(item['id'])
            if item['tag_name'] == tag:
                same_tag.append(item)
        if len(data) < 100:
            break
    else:
        raise RuntimeError('post-create release discovery incomplete')
    if same_tag != [value]:
        raise RuntimeError('reserved release not uniquely visible; reconcile before signing')
    check_tag()
    record['status'] = 'reserved'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', required=True)
    parser.add_argument('--expected-commit', required=True)
    parser.add_argument('--receipt', required=True)
    args = parser.parse_args()
    record = {'status': 'failed'}
    with open(args.receipt, 'x') as output:
        try:
            reserve(args.version, args.expected_commit, record)
        except Exception as error:
            record['error'] = str(error)
            raise
        finally:
            json.dump(record, output, sort_keys=True)
            output.write('\n'); output.flush(); os.fsync(output.fileno())


if __name__ == '__main__':
    main()
