"""Read-only contract for the observed native pkgbuild replacement metadata."""
import os
import stat
import xml.etree.ElementTree as ET

from artifact_modes import open_directory

LIMIT = 64 * 1024
BUNDLE_ID = "ai.sirsi.pantheon"


def validate(data, version, build, package_id=BUNDLE_ID, package_version=None):
    if len(data) > LIMIT or b"<!DOCTYPE" in data.upper() or b"<!ENTITY" in data.upper():
        raise ValueError("unsupported/bounded PackageInfo XML")
    root = ET.fromstring(data)
    if root.tag != "pkg-info" or any(root.get(key) != value for key, value in {
        "identifier": package_id, "version": package_version or version, "install-location": "/",
        "overwrite-permissions": "true", "relocatable": "false",
    }.items()):
        raise ValueError("PackageInfo package identity/version/public replacement mismatch")
    expected = {"payload", "bundle", "bundle-version", "upgrade-bundle", "update-bundle", "atomic-update-bundle", "strict-identifier", "relocate"}
    children = list(root)
    if len(children) != len(expected) or {child.tag for child in children} != expected:
        raise ValueError("PackageInfo unexpected/duplicate/missing native component marker")
    bundle = root.find("bundle")
    if bundle.attrib != {"path": "./Applications/Pantheon.app", "id": BUNDLE_ID, "CFBundleShortVersionString": version, "CFBundleVersion": build} or len(bundle):
        raise ValueError("PackageInfo exact app path/identity/version mismatch")
    for tag in ("bundle-version", "upgrade-bundle", "strict-identifier"):
        container = root.find(tag)
        members = list(container)
        if container.attrib or len(members) != 1 or members[0].tag != "bundle" or members[0].attrib != {"id": BUNDLE_ID} or len(members[0]):
            raise ValueError("PackageInfo missing exact sole " + tag + " membership")
    for tag in ("update-bundle", "atomic-update-bundle", "relocate"):
        container = root.find(tag)
        if len(container) or container.attrib or (container.text or "").strip():
            raise ValueError("PackageInfo conflicting " + tag + " membership")
    for element in root.iter():
        if (element.text or "").strip() or (element.tail or "").strip():
            raise ValueError("PackageInfo unexpected text")


def check(path, version, build):
    parent = open_directory(os.path.dirname(os.path.abspath(path)))
    try:
        fd = os.open(os.path.basename(path), os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=parent)
    finally:
        os.close(parent)
    with os.fdopen(fd, "rb") as stream:
        info = os.fstat(stream.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_size > LIMIT:
            raise ValueError("PackageInfo is not a bounded regular file")
        data = stream.read(LIMIT + 1)
        after = os.fstat(stream.fileno())
        if (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns) != (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns) or len(data) != info.st_size:
            raise ValueError("PackageInfo changed during read")
    validate(data, version, build)


if __name__ == "__main__":
    import argparse
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("path")
    parser.add_argument("version")
    parser.add_argument("build")
    args = parser.parse_args()
    try:
        check(args.path, args.version, args.build)
    except (OSError, ValueError, ET.ParseError) as error:
        parser.exit(1, "PackageInfo replacement contract refused: " + str(error) + "\n")
    print("PackageInfo fixed atomic upgrade metadata accepted")
