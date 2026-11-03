#!/usr/bin/env python3
"""Refuse another workflow run for a tag until operator reconciliation.

This is a bounded GitHub run check, not proof that historical custody is
absent. It does not replace the publisher's tag and asset readback.
"""
import json
import os
import re
import subprocess
import sys
from urllib.parse import urlencode


def main():
    tag = os.environ.get('GITHUB_REF_NAME', '')
    run = os.environ.get('GITHUB_RUN_ID', '')
    commit = sys.argv[1] if len(sys.argv) == 2 else ''
    if (not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?', tag) or
            not run.isdigit() or not re.fullmatch(r'[0-9a-f]{40}', commit)):
        raise SystemExit('exact run, tag and peeled commit required')
    query = urlencode({'branch': tag, 'event': 'push', 'per_page': 100, 'page': 1})
    endpoint = 'repos/SirsiMaster/sirsi-pantheon/actions/workflows/release.yml/runs?' + query
    result = subprocess.run(['gh', 'api', '--hostname', 'github.com', endpoint],
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30, check=False)
    if result.returncode or len(result.stdout) > 2 * 1024 * 1024:
        raise SystemExit('release run custody unknown; reconcile before signing')
    data = json.loads(result.stdout)
    runs = data.get('workflow_runs')
    if (not isinstance(runs, list) or type(data.get('total_count')) is not int or
            data['total_count'] != len(runs) or len(runs) > 100):
        raise SystemExit('release run custody listing incomplete; reconcile before signing')
    if (len(runs) != 1 or runs[0].get('id') != int(run) or runs[0].get('head_branch') != tag or
            runs[0].get('head_sha') != commit or runs[0].get('event') != 'push'):
        raise SystemExit('another or mismatched tag run exists; reconcile before signing')

    print('bounded single tag-run listing confirmed; historical custody is not inferred')


if __name__ == '__main__':
    main()
