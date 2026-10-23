#!/usr/bin/env python3
"""Classify the complete NUL-separated Git diff, not any matching path."""
import sys


def docs_only(data):
    paths = data.split(b"\0")
    if paths and paths[-1] == b"":
        paths.pop()
    return bool(paths) and all(
        path and (path.startswith(b"docs/") or path.startswith(b".thoth/")
                  or path.endswith(b".md")) for path in paths
    )


if __name__ == "__main__":
    print("true" if docs_only(sys.stdin.buffer.read()) else "false")
