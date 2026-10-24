#!/usr/bin/env python3
"""Publish the existing commercial pair only after draft asset byte readback.

No signing/build/credential provisioning occurs here. The existing release
builders and enrolled signer retain that authority. A published retry is read
only; this helper never overwrites or removes a release asset.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import tempfile

REPO = "SirsiMaster/sirsi-pantheon"
RELEASE_REPO = "github.com/" + REPO
TEAM = "9D382WV988"
REQUIREMENT = ('=anchor apple generic and certificate leaf[subject.OU] = "' + TEAM +
               '" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and '
               'certificate leaf[field.1.2.840.113635.100.6.1.13] exists')
MAX_ASSET_BYTES = 4 * 1024 ** 3


class Refusal(RuntimeError):
    pass


def execute(argv):
    return subprocess.run(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                          timeout=180, check=False)


def digest(path):
    # Retain the same regular-file descriptor while hashing; no FIFO or leaf
    # symlink open. This is ordinary cooperative runner custody, not hostile-UID
    # atomic protection across an eventual external gh upload.
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or not 0 < before.st_size <= MAX_ASSET_BYTES:
            raise Refusal("asset must be a nonempty bounded regular file")
        h, size = hashlib.sha256(), 0
        while True:
            block = os.read(fd, 1024 * 1024)
            if not block:
                break
            size += len(block)
            if size > MAX_ASSET_BYTES:
                raise Refusal("asset grew beyond byte bound")
            h.update(block)
        after = os.fstat(fd)
        if (before.st_size, before.st_mtime_ns, before.st_ctime_ns) != (
                after.st_size, after.st_mtime_ns, after.st_ctime_ns) or size != before.st_size:
            raise Refusal("asset changed while hashing")
        return {"sha256": h.hexdigest(), "bytes": size}
    finally:
        os.close(fd)


def publish(version, expected_commit, directory, app, record, run=execute):
    record.update(published=None, publish_attempted=False)
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?", version):
        raise Refusal("invalid release version")
    if not re.fullmatch(r"[0-9a-f]{40}", expected_commit):
        raise Refusal("exact peeled source commit required")
    tag = "v" + version
    names = ["SirsiPantheon-" + version + "-arm64." + ext for ext in ("dmg", "pkg")]
    paths = {name: Path(directory) / name for name in names}

    def checked(argv):
        result = run(argv)
        if result.returncode:
            raise Refusal("command failed: " + " ".join(argv[:2]))
        if len(result.stdout) + len(result.stderr) > 1024 * 1024:
            raise Refusal("command metadata exceeds bound")
        return result.stdout + result.stderr

    # The remote tag (annotated or lightweight) is the source identity, not a
    # mutable release targetCommitish branch-name projection.
    def check_tag():
        refs = checked(["git", "ls-remote", "--exit-code", "origin",
                        "refs/tags/" + tag, "refs/tags/" + tag + "^{}"]).decode().splitlines()
        remote = dict(line.split("\t", 1)[::-1] for line in refs)
        if remote.get("refs/tags/" + tag + "^{}", remote.get("refs/tags/" + tag)) != expected_commit:
            raise Refusal("remote tag does not peel to expected source")

    check_tag()

    expected = {name: digest(path) for name, path in paths.items()}
    record.update(schema="pantheon.release-publication.v1", tag=tag, commit=expected_commit,
                  assets=expected, verification=[], status="verifying", uploads_attempted=[])

    # Cryptographically reverify the already-built payload and staple tickets;
    # do not treat caller text or a signing receipt's prose as proof. These
    # reports are retained with the exact final asset hashes in the CI receipt.
    commands = [
        ["/usr/bin/codesign", "--verify", "--deep", "--strict", "-R", REQUIREMENT, str(app)],
        ["/usr/bin/codesign", "--verify", "--strict", "-R", REQUIREMENT, str(paths[names[0]])],
        ["/usr/sbin/pkgutil", "--check-signature", str(paths[names[1]])],
        *[["/usr/bin/xcrun", "stapler", "validate", str(paths[name])] for name in names],
    ]
    for argv in commands:
        report = checked(argv)
        if argv[0] == "/usr/sbin/pkgutil" and (
                b"signed by a developer certificate issued by Apple" not in report or
                not re.search(rb"Developer ID Installer: [^\r\n]+ \(9D382WV988\)", report)):
            raise Refusal("PKG lacks trusted Team Developer ID Installer verification")
        record["verification"].append({"argv": argv, "exit": 0,
                                       "report": report.decode(errors="replace")})

    def local_unchanged():
        if {name: digest(path) for name, path in paths.items()} != expected:
            raise Refusal("local verified pair changed; rebuild/review required")

    def view():
        response = run(["gh", "api", "--hostname", "github.com", "--include",
                        "repos/" + REPO + "/releases/tags/" + tag])
        if len(response.stdout) + len(response.stderr) > 1024 * 1024:
            raise Refusal("release metadata exceeds bound")
        # Only an explicit HTTP404 is absence. Network/auth failures never
        # become permission to create another release record.
        raw = response.stdout.replace(b"\r\n", b"\n")
        headers, separator, body = raw.partition(b"\n\n")
        status = re.match(rb"HTTP/\S+ ([0-9]{3})(?: |\n|$)", headers)
        code = int(status[1]) if status else 0
        explicit_404 = code == 404 or (not status and re.search(rb"\(HTTP 404\)(?:\r?\n|$)", response.stderr))
        if explicit_404 and response.returncode:
            record["published"] = False  # Observed absence, not a lookup failure.
            return None
        if not separator or code != 200 or response.returncode:
            raise Refusal("release lookup is unknown, not absent")
        value = json.loads(body)
        if value.get("tag_name") != tag or type(value.get("draft")) is not bool:
            raise Refusal("release tag/state mismatch")
        record["published"] = not value["draft"]
        assets = value.get("assets")
        if not isinstance(assets, list) or len(assets) > 2:
            raise Refusal("unexpected release asset set")
        seen = set()
        for asset in assets:
            name = asset.get("name")
            if name not in names or name in seen or asset.get("state") != "uploaded" or asset.get("size") != expected[name]["bytes"]:
                raise Refusal("duplicate, unexpected, incomplete or wrong-size release asset")
            seen.add(name)
        return value

    def readback(value, complete):
        assets = value["assets"]
        if complete and len(assets) != 2:
            raise Refusal("published/ready release must contain the exact complete pair")
        # Each read uses a fresh helper-owned directory; nothing is executed,
        # mounted, installed, or removed from the release. Temporary download
        # cleanup runs on every failure as well as success.
        with tempfile.TemporaryDirectory(prefix="pantheon-release-readback-") as temp:
            for asset in assets:
                name = asset["name"]
                checked(["gh", "release", "download", tag, "--repo", RELEASE_REPO,
                         "--pattern", name, "--dir", temp])
                if digest(Path(temp) / name) != expected[name]:
                    raise Refusal("remote asset bytes differ; never overwrite")
        return {asset["name"] for asset in assets}

    local_unchanged()
    release = view()
    if release is None:
        checked(["gh", "release", "create", tag, "--repo", RELEASE_REPO, "--draft", "--verify-tag",
                 "--target", expected_commit, "--title", "Sirsi Pantheon " + version,
                 "--generate-notes"])
        release = view()
        if release is None or not release["draft"]:
            raise Refusal("new staging record is not a draft")
    if not release["draft"]:
        record["published"] = True
        readback(release, True)
        check_tag()
        record.update(status="verified_existing_published", published=True, remote_mutation=False)
        return

    present = readback(release, False)
    record["draft_seen"] = True
    for name in names:
        if name not in present:
            local_unchanged()
            # No clobber, asset delete, or published-asset replacement path.
            current = view()
            if current is None or not current["draft"]:
                raise Refusal("draft changed before upload; recheck published pair")
            record["uploads_attempted"].append(name)
            checked(["gh", "release", "upload", tag, "--repo", RELEASE_REPO, str(paths[name])])
    release = view()
    if release is None:
        raise Refusal("staged release disappeared before paired readback")
    readback(release, True)
    local_unchanged()
    record["prepublication_readback"] = expected
    if release["draft"]:
        check_tag()
        # An interrupted request can have succeeded remotely. Preserve that
        # uncertainty rather than claiming a failed readback means unpublished.
        record.update(publish_attempted=True, published=None)
        checked(["gh", "release", "edit", tag, "--repo", RELEASE_REPO, "--draft=false"])
    final = view()
    if final is None or final["draft"]:
        raise Refusal("publish was not read back")
    record["published"] = True
    readback(final, True)
    check_tag()
    record.update(status="published_verified_pair", published=True, remote_mutation=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--expected-commit", required=True)
    parser.add_argument("--directory", default="bin")
    parser.add_argument("--app", default="Pantheon.app")
    parser.add_argument("--receipt", required=True)
    args = parser.parse_args()
    os.environ["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
    record = {"schema": "pantheon.release-publication.v1", "status": "failed",
              "published": None, "tag": "v" + args.version, "commit": args.expected_commit}
    # Exclusive receipt creation refuses accidental reuse of another attempt.
    with open(args.receipt, "x") as receipt:
        try:
            publish(args.version, args.expected_commit, args.directory, args.app, record)
        except Exception as error:
            record.update(status="failed", error=str(error))
            raise
        finally:
            record["time"] = datetime.datetime.now(datetime.timezone.utc).isoformat()
            json.dump(record, receipt, indent=2)
            receipt.write("\n")
            receipt.flush()
            os.fsync(receipt.fileno())


if __name__ == "__main__":
    main()
