#!/usr/bin/env python3
"""Publish the exact commercial asset set only after draft byte readback.

No signing/build/credential provisioning occurs here. The existing release
builders and enrolled signer retain that authority. A published retry is read
only; this helper never overwrites or removes a release asset.

New drafts retain the bounded creation response ID and use selected-ID GETs,
never a post-create pagination walk. Unknown creation outcomes stop for custody
reconciliation. Inert contract fixtures: scripts/test_release_publication.py.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import selectors
import stat
import subprocess
import tempfile
import time
from urllib.parse import quote

REPO = "SirsiMaster/sirsi-pantheon"
RELEASE_REPO = "github.com/" + REPO
TEAM = "9D382WV988"
REQUIREMENT = ('=anchor apple generic and certificate leaf[subject.OU] = "' + TEAM +
               '" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and '
               'certificate leaf[field.1.2.840.113635.100.6.1.13] exists')
MAX_ASSET_BYTES = 4 * 1024 ** 3
MAX_RELEASE_PAGES = 10
RELEASE_PAGE_SIZE = 100


class Refusal(RuntimeError):
    pass


def execute(argv):
    return subprocess.run(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                          timeout=180, check=False)


def download_asset(asset_id, path, expected_bytes):
    # Asset IDs are server-assigned release metadata, not caller URLs. Stream
    # bytes to an exclusive owned file rather than buffering a large asset.
    if type(expected_bytes) is not int or not 0 < expected_bytes <= MAX_ASSET_BYTES:
        raise Refusal("invalid selected asset size")
    process = subprocess.Popen(["gh", "api", "--hostname", "github.com", "-H",
                                "Accept: application/octet-stream",
                                f"repos/{REPO}/releases/assets/{asset_id}"],
                               stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    deadline, size = time.monotonic() + 180, 0
    try:
        with open(path, "xb") as output, selectors.DefaultSelector() as selector:
            selector.register(process.stdout, selectors.EVENT_READ)
            while True:
                remaining = deadline - time.monotonic()
                if remaining <= 0 or not selector.select(remaining):
                    raise Refusal("selected release asset download timed out")
                block = os.read(process.stdout.fileno(), min(1024 * 1024, expected_bytes - size + 1))
                if not block:
                    break
                size += len(block)
                if size > expected_bytes:
                    raise Refusal("selected release asset exceeded expected size")
                output.write(block)
            output.flush()
            os.fsync(output.fileno())
        remaining = max(0, deadline - time.monotonic())
        if process.wait(timeout=remaining) or size != expected_bytes:
            raise Refusal("selected release asset download failed or was incomplete")
    except BaseException:
        if process.poll() is None:
            process.kill()
            process.wait()
        raise
    finally:
        process.stdout.close()


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


def verify_guard_image(path, expected, run=execute):
    """Inspect a signed image's exact two payloads; never execute them.

    The expected hashes and identifiers must come from the retained, separately
    verified signing/package custody of this same release attempt. A failed or
    ambiguous attach/detach is a custody stop, not permission to retry.
    """
    names = ("sirsi-display-power-guard", "sirsi-pretoolguard-install")
    if not isinstance(expected, dict) or set(expected) != set(names):
        raise Refusal("exact guard payload manifest required")
    for name in names:
        item = expected[name]
        if (not isinstance(item, dict) or set(item) != {"sha256", "identifier"} or
                not isinstance(item["sha256"], str) or
                not re.fullmatch(r"[0-9a-f]{64}", item["sha256"]) or
                not isinstance(item["identifier"], str) or
                not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.-]*", item["identifier"])):
            raise Refusal("invalid guard payload manifest")

    # Retain the directory on an unknown mount outcome; never try to clean up
    # a mount whose identity has not been established by the successful attach.
    mount = Path(tempfile.mkdtemp(prefix="pantheon-guard-publication-mount-"))
    device = None
    metadata_path = None
    run_id, attempt_id = os.environ.get("GITHUB_RUN_ID", "local"), os.environ.get("GITHUB_RUN_ATTEMPT", "0")
    if not re.fullmatch(r"(?:[0-9]+|local)", run_id) or not re.fullmatch(r"[0-9]+", attempt_id):
        raise Refusal("invalid run identity for mount custody")
    def retain_attach(stdout, stderr):
        # Preserve bounded partial output even for timeout or nonzero exit; it
        # may identify devices that need operator reconciliation.
        fd, name = tempfile.mkstemp(prefix="pantheon-guard-attach-" + run_id + "-" + attempt_id + "-",
                                    suffix=".plist", dir=os.environ.get("RUNNER_TEMP"))
        raw = stdout if isinstance(stdout, bytes) else (stdout or "").encode()
        err = stderr if isinstance(stderr, bytes) else (stderr or "").encode()
        with os.fdopen(fd, "wb") as retained:
            retained.write(raw[:1024 * 1024])
            retained.flush()
            os.fsync(retained.fileno())
        if err:
            with open(name + ".stderr", "xb") as retained:
                retained.write(err[:1024 * 1024])
                retained.flush()
                os.fsync(retained.fileno())
        return name
    try:
        try:
            result = run(["/usr/bin/hdiutil", "attach", "-plist", "-readonly", "-nobrowse",
                          "-mountpoint", str(mount), str(path)])
        except subprocess.TimeoutExpired as error:
            try:
                metadata_path = retain_attach(error.stdout, error.stderr)
            except OSError as retain_error:
                raise Refusal("attach timed out and metadata retention failed; inspect " + str(mount)) from retain_error
            raise Refusal("guard image attach outcome unknown; inspect " + str(mount) + " and " + metadata_path) from error
        except OSError as error:
            raise Refusal("guard image attach outcome unknown; inspect " + str(mount)) from error
        try:
            metadata_path = retain_attach(result.stdout, result.stderr)
        except OSError as error:
            raise Refusal("attachment metadata retention failed; inspect " + str(mount)) from error
        if result.returncode:
            raise Refusal("guard image attach failed or is ambiguous; inspect " + str(mount) + " and " + metadata_path)
        try:
            if len(result.stdout) > 1024 * 1024:
                raise ValueError("oversized attachment metadata")
            topology = plistlib.loads(result.stdout)
            entities = topology["system-entities"]
            if (not isinstance(entities, list) or not entities or
                    any(not isinstance(entity, dict) for entity in entities)):
                raise ValueError("missing image entities")
            devices = [entity.get("dev-entry") for entity in entities if isinstance(entity, dict) and entity.get("dev-entry")]
            roots = {re.fullmatch(r"(/dev/disk[0-9]+)(?:s[0-9]+)*", entry).group(1)
                     for entry in devices}
            if not devices or len(roots) != 1:
                raise ValueError("ambiguous image devices")
            candidate = roots.pop()
            matching = [entity for entity in entities if entity.get("mount-point") == str(mount)]
            if len(matching) != 1 or matching[0].get("dev-entry") not in devices:
                raise Refusal("guard image mount/device association unknown; inspect " + str(mount) + " and " + str(metadata_path))
            mounted_device = matching[0]["dev-entry"]
            device = candidate
            mounts = [entity["mount-point"] for entity in entities
                      if isinstance(entity, dict) and "mount-point" in entity]
            if mounts != [str(mount)]:
                raise Refusal("guard image has unexpected mounted volumes")
            if mounted_device not in devices or set(devices) != {device, mounted_device}:
                raise Refusal("guard image has unexpected unmounted device entities")
        except (ValueError, TypeError, KeyError, AttributeError, plistlib.InvalidFileException) as error:
            raise Refusal("guard image attachment topology unknown; inspect " + str(mount) +
                          " and " + str(metadata_path)) from error
        if set(os.listdir(mount)) != set(names):
            raise Refusal("guard image contains unexpected payloads")
        for name in names:
            payload = mount / name
            info = os.lstat(payload)
            if (not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or
                    stat.S_IMODE(info.st_mode) != 0o755):
                raise Refusal("guard payload must be a single-link mode-0755 regular file")
            if digest(payload)["sha256"] != expected[name]["sha256"]:
                raise Refusal("guard payload does not match retained signing custody")
            requirement = '=identifier "' + expected[name]["identifier"] + '" and ' + REQUIREMENT[1:]
            checks = [["/usr/bin/codesign", "--verify", "--strict", "-R", requirement, str(payload)],
                      ["/usr/bin/python3", "-B", str(Path(__file__).with_name("verify-pretoolguard-entitlements.py")), str(payload)]]
            for argv in checks:
                result = run(argv)
                if result.returncode or len(result.stdout) + len(result.stderr) > 1024 * 1024:
                    raise Refusal("guard payload signature or entitlements not verified")
            details = run(["/usr/bin/codesign", "-d", "--verbose=4", str(payload)])
            report = details.stdout + details.stderr
            if (details.returncode or len(report) > 1024 * 1024 or
                    not re.search(rb"(?m)^Timestamp=", report) or
                    not re.search(rb"(?m)^CodeDirectory .*flags=.*\(.*runtime.*\)", report)):
                raise Refusal("guard payload lacks verified timestamp or hardened runtime")
    finally:
        if device is not None:
            try:
                detached = run(["/usr/bin/hdiutil", "detach", device])
            except (OSError, subprocess.TimeoutExpired) as error:
                raise Refusal("guard image detach outcome unknown; inspect " + str(mount)) from error
            if detached.returncode:
                raise Refusal("guard image detach failed; inspect " + str(mount))
            os.rmdir(mount)
    return metadata_path


def publish(version, expected_commit, directory, app, record, run=execute, include_guard=False,
            guard_manifest=None, reserved_id=None, download=download_asset):
    record.update(published=None, publish_attempted=False, creation_attempted=False)
    if include_guard and guard_manifest is None:
        raise Refusal("guard release requires verified payload custody manifest")
    if include_guard and (type(reserved_id) is not int or reserved_id <= 0):
        raise Refusal("guard release requires the exact reserved release ID")
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?", version):
        raise Refusal("invalid release version")
    if not re.fullmatch(r"[0-9a-f]{40}", expected_commit):
        raise Refusal("exact peeled source commit required")
    tag = "v" + version
    names = ["SirsiPantheon-" + version + "-arm64." + ext for ext in ("dmg", "pkg")]
    if include_guard:
        names.append("SirsiPantheon-" + version + "-arm64-pretoolguard.dmg")
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
        *([["/usr/bin/codesign", "--verify", "--strict", "-R", REQUIREMENT, str(paths[names[2]])]]
          if include_guard else []),
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

    if include_guard:
        image_details = checked(["/usr/bin/codesign", "-d", "--verbose=4", str(paths[names[2]])])
        if not re.search(rb"(?m)^Timestamp=", image_details):
            raise Refusal("guard image secure timestamp not verified")
        record["verification"].append({"argv": ["/usr/bin/codesign", "-d", "--verbose=4", str(paths[names[2]])],
                                       "exit": 0, "report": image_details.decode(errors="replace")})
        record["guard_attachment_metadata"] = verify_guard_image(paths[names[2]], guard_manifest, run)
        if digest(paths[names[2]]) != expected[names[2]]:
            raise Refusal("guard image changed during payload verification")
        record["guard_payloads"] = guard_manifest

    def local_unchanged():
        if {name: digest(path) for name, path in paths.items()} != expected:
            raise Refusal("local verified asset set changed; rebuild/review required")

    if reserved_id is not None and (type(reserved_id) is not int or reserved_id <= 0):
        raise Refusal("invalid reserved release ID")
    selected_id = reserved_id
    if selected_id is not None:
        record["release_id"] = selected_id

    def release_api(endpoint, allow_missing=False, project_discovery=False):
        argv = ["gh", "api", "--hostname", "github.com", "--include"]
        if project_discovery:
            if allow_missing or not re.fullmatch(r"\?per_page=100&page=(?:[1-9]|10)", endpoint):
                raise Refusal("projection is restricted to bounded discovery lists")
            argv += ["--jq", "map({id,tag_name,draft})"]
        argv.append("repos/" + REPO + "/releases" +
                    ("" if endpoint.startswith("?") else "/") + endpoint)
        response = run(argv)
        if len(response.stdout) + len(response.stderr) > 1024 * 1024:
            raise Refusal("release metadata exceeds bound")
        # A tag404 is NOT draft absence: private drafts may only appear in the
        # authenticated repository list and exact-ID lookup. Other failures
        # cannot authorize creation, and selected-ID404 never means recreate.
        raw = response.stdout.replace(b"\r\n", b"\n")
        headers, separator, body = raw.partition(b"\n\n")
        status = re.match(rb"HTTP/\S+ ([0-9]{3})(?: |\n|$)", headers)
        code = int(status[1]) if status else 0
        explicit_404 = code == 404 or (not status and re.search(rb"\(HTTP 404\)(?:\r?\n|$)", response.stderr))
        if explicit_404 and response.returncode:
            if allow_missing:
                return None
            raise Refusal("selected release/discovery unavailable; never recreate")
        if not separator or code != 200 or response.returncode:
            raise Refusal("release lookup is unknown, not absent")
        return json.loads(body)

    def validate_release(value, expected_id=None):
        if (not isinstance(value, dict) or value.get("tag_name") != tag or
                type(value.get("id")) is not int or value["id"] <= 0 or
                type(value.get("draft")) is not bool):
            raise Refusal("release tag/state mismatch")
        if expected_id is not None and value["id"] != expected_id:
            raise Refusal("selected release identity changed")
        record["published"] = not value["draft"]
        assets = value.get("assets")
        if not isinstance(assets, list) or len(assets) > len(names):
            raise Refusal("unexpected release asset set")
        seen = set()
        for asset in assets:
            name = asset.get("name")
            if name not in names or name in seen or asset.get("state") != "uploaded" or asset.get("size") != expected[name]["bytes"]:
                raise Refusal("duplicate, unexpected, incomplete or wrong-size release asset")
            if include_guard and (type(asset.get("id")) is not int or asset["id"] <= 0):
                raise Refusal("guard release asset ID missing")
            seen.add(name)
        return value

    def discover():
        # Complete bounded pagination is required even after a match, so a
        # second same-tag draft cannot be silently selected by list order.
        matches, ids = [], set()
        for page in range(1, MAX_RELEASE_PAGES + 1):
            values = release_api("?per_page=" + str(RELEASE_PAGE_SIZE) + "&page=" + str(page), project_discovery=True)
            if not isinstance(values, list) or len(values) > RELEASE_PAGE_SIZE:
                raise Refusal("invalid release discovery page")
            for value in values:
                if (not isinstance(value, dict) or type(value.get("id")) is not int or
                        value["id"] <= 0 or value["id"] in ids or
                        not isinstance(value.get("tag_name"), str) or
                        type(value.get("draft")) is not bool):
                    raise Refusal("ambiguous release discovery metadata")
                ids.add(value["id"])
                if value["tag_name"] == tag:
                    matches.append(value)
            if len(values) < RELEASE_PAGE_SIZE:
                if len(matches) > 1:
                    raise Refusal("duplicate exact-tag release records; never select or create")
                return matches[0] if matches else None
        raise Refusal("release discovery incomplete at bound; absence unproved")

    def view():
        nonlocal selected_id
        if selected_id is not None:
            value = validate_release(release_api(str(selected_id)), selected_id)
            return value
        value = release_api("tags/" + tag, allow_missing=True)
        if value is None:
            value = discover()
            if value is None:
                record["published"] = False  # Complete authenticated absence.
                return None
            # List metadata alone is insufficient for an upload/publish choice.
            listed_id = value["id"]
            value = validate_release(release_api(str(listed_id)), listed_id)
        else:
            value = validate_release(value)
        selected_id = value["id"]
        record["release_id"] = selected_id
        return value

    def readback(value, complete):
        assets = value["assets"]
        if complete and len(assets) != len(names):
            raise Refusal("published/ready release must contain the exact complete asset set")
        # Each read uses a fresh helper-owned directory; nothing is executed,
        # mounted, installed, or removed from the release. Temporary download
        # cleanup runs on every failure as well as success.
        with tempfile.TemporaryDirectory(prefix="pantheon-release-readback-") as temp:
            for asset in assets:
                name = asset["name"]
                if include_guard:
                    download(asset["id"], Path(temp) / name, expected[name]["bytes"])
                else:
                    checked(["gh", "release", "download", tag, "--repo", RELEASE_REPO,
                             "--pattern", name, "--dir", temp])
                if digest(Path(temp) / name) != expected[name]:
                    raise Refusal("remote asset bytes differ; never overwrite")
        return {asset["name"] for asset in assets}

    local_unchanged()
    release = view()
    if release is None:
        check_tag()
        # Absence was proved before the mutation. The creation response is the
        # new identity: another paginated list is neither that response nor a
        # stable snapshot after insertion. Unknown outcomes never recreate.
        record.update(creation_attempted=True, creation_outcome="unknown", published=None)
        created = checked([
            "gh", "api", "--hostname", "github.com", "--include", "--method", "POST",
            "--jq", "{id,tag_name,draft}", "-f", "tag_name=" + tag,
            "-f", "target_commitish=" + expected_commit, "-f", "name=Sirsi Pantheon " + version,
            "-F", "draft=true", "-F", "generate_release_notes=true",
            "repos/" + REPO + "/releases"])
        headers, separator, body = created.replace(b"\r\n", b"\n").partition(b"\n\n")
        if not separator or not re.match(rb"HTTP/\S+ 201(?: |\n|$)", headers):
            raise Refusal("draft creation outcome unknown; never rediscover or recreate")
        try:
            identity = json.loads(body)
        except (ValueError, UnicodeDecodeError) as error:
            raise Refusal("draft creation identity unreadable; never rediscover or recreate") from error
        if (not isinstance(identity, dict) or set(identity) != {"id", "tag_name", "draft"} or
                type(identity.get("id")) is not int or
                identity["id"] <= 0 or identity.get("tag_name") != tag or
                identity.get("draft") is not True):
            raise Refusal("draft creation identity invalid; never rediscover or recreate")
        selected_id = identity["id"]
        record.update(release_id=selected_id, creation_outcome="draft_identity_received")
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
            if include_guard:
                checked(["gh", "api", "--hostname", "github.com", "--method", "POST",
                         "-H", "Content-Type: application/octet-stream", "--input", str(paths[name]),
                         f"https://uploads.github.com/repos/{REPO}/releases/{selected_id}/assets?name={quote(name, safe='')}"])
            else:
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
        if include_guard:
            checked(["gh", "api", "--hostname", "github.com", "--method", "PATCH", "-F", "draft=false",
                     f"repos/{REPO}/releases/{selected_id}"])
        else:
            checked(["gh", "release", "edit", tag, "--repo", RELEASE_REPO, "--draft=false"])
    final = view()
    if final is None or final["draft"]:
        raise Refusal("publish was not read back")
    record["published"] = True
    readback(final, True)
    check_tag()
    record.update(status="published_verified_set" if include_guard else "published_verified_pair",
                  published=True, remote_mutation=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--expected-commit", required=True)
    parser.add_argument("--directory", default="bin")
    parser.add_argument("--app", default="Pantheon.app")
    parser.add_argument("--include-guard", action="store_true",
                        help="require the versioned signed guard image in the exact release asset set")
    parser.add_argument("--guard-manifest", help="retained payload custody JSON for this exact release attempt")
    parser.add_argument("--reservation", help="exact retained draft reservation receipt")
    parser.add_argument("--receipt", required=True)
    args = parser.parse_args()
    os.environ["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
    record = {"schema": "pantheon.release-publication.v1", "status": "failed",
              "published": None, "tag": "v" + args.version, "commit": args.expected_commit}
    # Exclusive receipt creation refuses accidental reuse of another attempt.
    with open(args.receipt, "x") as receipt:
        try:
            reserved_id = None
            if args.reservation:
                fd = os.open(args.reservation, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
                try:
                    info = os.fstat(fd)
                    if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= 16384:
                        raise Refusal("invalid reservation receipt")
                    contents = os.read(fd, 16385)
                    after = os.fstat(fd)
                    if (len(contents) != info.st_size or
                            (info.st_size, info.st_mtime_ns, info.st_ctime_ns) !=
                            (after.st_size, after.st_mtime_ns, after.st_ctime_ns)):
                        raise Refusal("reservation receipt changed during read")
                    reservation = json.loads(contents)
                finally:
                    os.close(fd)
                if (reservation.get("status") != "reserved" or reservation.get("tag") != "v" + args.version or
                        reservation.get("commit") != args.expected_commit or type(reservation.get("release_id")) is not int or
                        reservation["release_id"] <= 0):
                    raise Refusal("reservation receipt identity mismatch")
                reserved_id = reservation["release_id"]
            guard_manifest = None
            if args.include_guard:
                if not args.guard_manifest:
                    raise Refusal("guard manifest is required")
                fd = os.open(args.guard_manifest, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
                try:
                    before = os.fstat(fd)
                    if not stat.S_ISREG(before.st_mode) or not 0 < before.st_size <= 16384:
                        raise Refusal("guard manifest must be a bounded regular file")
                    content = os.read(fd, 16385)
                    after = os.fstat(fd)
                    if (len(content) != before.st_size or
                            (before.st_size, before.st_mtime_ns, before.st_ctime_ns) !=
                            (after.st_size, after.st_mtime_ns, after.st_ctime_ns)):
                        raise Refusal("guard manifest changed during read")
                    guard_manifest = json.loads(content)
                finally:
                    os.close(fd)
            elif args.guard_manifest:
                raise Refusal("guard manifest requires --include-guard")
            publish(args.version, args.expected_commit, args.directory, args.app, record,
                    include_guard=args.include_guard, guard_manifest=guard_manifest, reserved_id=reserved_id)
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
