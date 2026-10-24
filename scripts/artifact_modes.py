#!/usr/bin/env python3
"""Build-staging metadata only; never installed into the Python-free payload."""
import argparse
import os
from pathlib import Path
import stat

BUNDLE_EXECUTABLES = {"Contents/MacOS/sirsi", "Contents/MacOS/sirsi-menubar"}
APP_PREFIX = "Applications/Pantheon.app"
SYSTEM_CLI = "usr/local/libexec/sirsi-pantheon/sirsi"
DIRECTORY_FLAGS = os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_NONBLOCK


def open_directory(path):
    """Reject links in every component, not merely at the supplied root leaf."""
    parts = Path(os.path.abspath(path)).parts
    current = os.open("/", DIRECTORY_FLAGS)
    try:
        for part in parts[1:]:
            following = os.open(part, DIRECTORY_FLAGS, dir_fd=current)
            os.close(current)
            current = following
        return current
    except BaseException:
        os.close(current)
        raise


def identity(info):
    return info.st_dev, info.st_ino, stat.S_IFMT(info.st_mode)


def bundle_mode(relative, directory):
    if directory:
        if relative in ("", "Contents", "Contents/MacOS", "Contents/Resources", "Contents/_CodeSignature") or relative.startswith(("Contents/Resources/", "Contents/_CodeSignature/")):
            return 0o755
    elif relative in BUNDLE_EXECUTABLES:
        return 0o755
    elif relative in ("Contents/Info.plist", "Contents/PkgInfo", "Contents/CodeResources") or relative.startswith(("Contents/Resources/", "Contents/_CodeSignature/")):
        return 0o644
    raise ValueError("unexpected bundle entry: " + relative)


def expected_mode(relative, directory, payload):
    if not payload:
        return bundle_mode(relative, directory)
    if relative == APP_PREFIX or relative.startswith(APP_PREFIX + "/"):
        return bundle_mode(relative[len(APP_PREFIX):].lstrip("/"), directory)
    if directory and relative in ("", "Applications", "usr", "usr/local", "usr/local/libexec", "usr/local/libexec/sirsi-pantheon"):
        return 0o755
    if not directory and relative == SYSTEM_CLI:
        return 0o755
    raise ValueError("unexpected package payload entry: " + relative)


def normalize(path, payload=False, check=False):
    root = open_directory(path)
    manifest = {}
    try:
        root_info = os.fstat(root)

        def visit(fd, relative, depth):
            if depth > 32 or len(manifest) >= 4096:
                raise ValueError("staging entry/depth bound exceeded")
            info = os.fstat(fd)
            if info.st_uid != os.geteuid() or info.st_dev != root_info.st_dev:
                raise ValueError("staging entry not owned on root device")
            directory = stat.S_ISDIR(info.st_mode)
            if not directory and (not stat.S_ISREG(info.st_mode) or info.st_nlink != 1):
                raise ValueError("nonregular or hard-linked staging entry")
            mode = expected_mode(relative, directory, payload)
            manifest[relative] = (info, mode)
            if check and stat.S_IMODE(info.st_mode) != mode:
                raise ValueError("public mode mismatch: " + relative)
            if directory:
                with os.scandir(fd) as entries:
                    for entry in entries:
                        child_relative = relative + "/" + entry.name if relative else entry.name
                        observed = os.stat(entry.name, dir_fd=fd, follow_symlinks=False)
                        if not (stat.S_ISDIR(observed.st_mode) or stat.S_ISREG(observed.st_mode)):
                            raise ValueError("link/special staging entry: " + child_relative)
                        flags = DIRECTORY_FLAGS if stat.S_ISDIR(observed.st_mode) else os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK
                        child = os.open(entry.name, flags, dir_fd=fd)
                        try:
                            if identity(observed) != identity(os.fstat(child)):
                                raise ValueError("staging entry replaced during preflight")
                            visit(child, child_relative, depth + 1)
                        finally:
                            os.close(child)

        # Full namespace/type/owner preflight before ANY mode mutation. Retain
        # only bounded metadata, then reopen each component with nofollow and
        # compare its recorded identity. No recursive chmod/path traversal.
        visit(root, "", 0)
        prefix = APP_PREFIX + "/" if payload else ""
        required = {prefix + item for item in BUNDLE_EXECUTABLES | {"Contents/Info.plist", "Contents/PkgInfo"}}
        if payload:
            required.add(SYSTEM_CLI)
        if not required.issubset(manifest):
            raise ValueError("required public artifact entry missing")
        if check:
            return len(manifest)
        for relative, (original, mode) in manifest.items():
            current = os.dup(root)
            try:
                traversed = ""
                parts = relative.split("/") if relative else []
                for index, part in enumerate(parts):
                    traversed = traversed + "/" + part if traversed else part
                    recorded = manifest[traversed][0]
                    flags = DIRECTORY_FLAGS if index < len(parts) - 1 or stat.S_ISDIR(recorded.st_mode) else os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK
                    following = os.open(part, flags, dir_fd=current)
                    os.close(current)
                    current = following
                    if identity(recorded) != identity(os.fstat(current)):
                        raise ValueError("staging component substituted")
                actual = os.fstat(current)
                if identity(actual) != identity(original) or actual.st_uid != os.geteuid():
                    raise ValueError("staging entry substituted")
                os.fchmod(current, mode)
                if stat.S_IMODE(os.fstat(current).st_mode) != mode:
                    raise ValueError("mode readback failed")
            finally:
                os.close(current)
        return len(manifest)
    finally:
        os.close(root)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--bundle")
    group.add_argument("--payload")
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    try:
        count = normalize(args.payload or args.bundle, payload=bool(args.payload), check=args.check)
    except (OSError, ValueError) as error:
        parser.exit(1, "artifact modes refused: " + str(error) + "\n")
    print("artifact public modes accepted entries=" + str(count))


if __name__ == "__main__":
    main()
