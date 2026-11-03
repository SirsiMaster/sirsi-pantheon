# Bounded pretoolguard installer (source candidate)

Classification: platform-foundation. This is **unbuilt, untested, unreviewed
source**, not an authorization to install. No deployment, signing, notarization,
or production qualification is performed by this change.

The exact selectors are `codex-user`, `claude-user`, and `codex-system`. User
targets resolve from the real non-root UID's account-database home, not HOME,
SUDO_USER or a supplied path. Real and effective UID must match. The system
selector requires real/effective root and resolves to
`/private/etc/codex/hooks/sirsi-display-power-guard` (the physical form of
`/etc/codex/hooks/...` on macOS). Traversed directories must be non-symlink
directories owned by root or the target UID and not group/world writable.
No automatic privilege elevation is provided.

## Contract

`-action install` requires `-target`, an absolute `-artifact`,
`-expected-current` (identity JSON), `-expected-original-provenance` (JSON), `-expected-replacement-sha256`,
`-expected-code-identifier`, and `-expected-team-id`. Identity JSON has fields
`device`, `inode`, `uid`, `gid`, `mode` (a JSON decimal number, e.g. 493 for
0755), and lowercase `sha256`. These are independent operator-supplied
expectations for the particular target, not an inventory or an authorization.
The original provenance proof is an independent expectation, for example
`{"provenance_present":true,"provenance_sha256":"<64 lowercase hex digits>"}`
or `{"provenance_present":false,"provenance_sha256":""}`. It must match
the original's readback before a transaction directory is created; an observed
value is not substituted for a missing operator expectation.
The artifact must pass SHA-256 readback and `/usr/bin/codesign --verify --strict`
with the specified identifier, Apple generic anchor, and certificate team.
Notarization, artifact provenance, and operator authority remain separate.

The installer creates a random, no-clobber, mode-0700 directory in the target
directory and stages a new regular file there; it syncs the object and journal,
checks the original and staged identities, journals the prepared state, then
uses Darwin `RENAME_SWAP`. The displaced original **itself** stays at `object`
in the private directory: its inode and metadata, including ACLs and extended
attributes, are not reconstructed by a copy. Both names are read back and
matched by identity and digest, directories are synced, and the installed record
is appended and synced. The replacement gets the original UID, GID and mode;
other metadata is not copied. The transaction ID
and directory must be successfully written to stderr before staging; a failed
or short write stops immediately, possibly leaving an empty transaction
directory. This is not a guarantee that the recipient durably stored the output.
A success record includes the exact
installed and backup identities. Both files must have zero flags, an absent
extended ACL, and no xattrs other than `com.apple.provenance`. The
provenance attribute is optional on each inode; if present it must be readable
and at most 1 MiB. Its presence and SHA-256 are recorded separately for original
and replacement in the version 2 journal, and checked against fresh readbacks
before and after each swap and during recovery. Unknown, unreadable, changing or
oversized attributes and unsupported or ambiguous metadata queries fail closed.

The original inode, including its exact provenance bytes, is retained as the
backup; the installer never sets, removes, copies, or overwrites provenance. The
new inode may independently have system-assigned provenance or none: it is not
required to match the original, nor is the artifact's provenance copied. A
system change to either recorded value after preparation blocks completion or
recovery for operator investigation; it is not silently accepted. The hash
detects changes subject to SHA-256's collision assumptions, but is not proof of
the provenance attribute's meaning, authenticity, or authorization. The check
depends on the OS exposing the attribute in descriptor-based listing and reads;
hidden or inaccessible metadata cannot be proved retained by this mechanism.
Metadata
other than the stated UID, GID, mode, flags, ACL and xattr policy is not promised.

`-action rollback -target <selector> -transaction <id> -expected-current
'<identity JSON>' -expected-backup '<identity JSON>'` requires both identities
to match an `installed` or `rollback-aborted` journal record and fresh readbacks.
It journals rollback preparation, exchanges the original inodes, verifies both
names, syncs the directories, and records `rolled-back`. The replacement is
retained in the private directory. Neither direction deletes retained files.

## Recovery and limitations

The append-only journal records prepared and terminal phases. A failure or crash
can leave a swap uncertain. **Stop; retain both objects and the journal.** An
authorized operator can invoke `-action recover -target <selector> -transaction
<id> -expected-current '<identity JSON>' -expected-backup '<identity JSON>'`,
where the two proofs describe the actual target and retained object, respectively.
Recovery checks the complete journal sequence and exact identities, verifies the
two names and metadata, syncs both directories, rechecks the identities, and
durably appends the observed outcome. It never swaps or infers that a crash means
the operation succeeded: install-prepared becomes installed or install-aborted;
rollback-prepared becomes rolled-back or rollback-aborted. A rollback-aborted
transaction can be explicitly retried while capacity permits. The journal is
limited to eight records; every prepared append reserves a terminal record and
byte capacity, so another rollback attempt is refused before preparation when
there is insufficient room. For an already complete `installed`, `install-aborted`,
`rollback-aborted`, or `rolled-back` record, recovery checks the corresponding
identity and metadata proofs, syncs both directories and the journal, rechecks
the record and objects, and acknowledges the existing phase without appending.
A torn or malformed journal, mismatched
identity, failed metadata check, or failed sync requires separate operator
intervention; no journal repair is attempted. An append or sync error can itself
leave an uncertain outcome. An error after a swap is not a promise the target
was unchanged. Version 1 journals lack provenance proofs and are rejected;
they require separate manual investigation, not automatic migration or recovery.

This protocol assumes cooperative same-UID writers across the whole operation;
there is no enforced lock, compare-and-swap of the expected inode, or protection
against a hostile same-UID writer. The owner explicitly accepts this cooperative-
writer limitation. It does not establish protection against a
privileged writer, hostile mounts, root compromise, ACL-based directory writes,
or all network/filesystem behavior. Directory ownership/mode checks do not
audit ACLs. Provenance preservation follows from retaining the same inode,
subject to filesystem behavior, and the recorded presence and digest detect
observed changes. In-place changes by
noncooperating writers are not prevented. Apple `F_FULLFSYNC`, directory `fsync`,
and swap durability/availability need qualification on the actual OS and
filesystem; unsupported operations fail closed, potentially after a swap. The
signed artifact's own ACLs/xattrs are not copied. The Darwin extended-security
query, its no-ACL response layout, and reading protected provenance have not
been qualified on the target OS or filesystem and may reject even the observed
targets; no successful installation is claimed until independently verified.
The ACL check issues a descriptor-based, single extended-security request
without `ATTR_CMN_RETURNED_ATTRS` or `FSOPT_PACK_INVAL_ATTRS`, and requests
full-size reporting. Apple's published XNU compatibility path returns an error
for unsupported requested vnode attributes and packs a supported null ACL as a
zero-length reference. The parser accepts only that exact 12-byte layout or a
well-formed no-ACL sentinel; empty ACLs, nonempty ACLs, malformed/truncated
responses, and syscall errors are rejected. This interpretation depends on that
kernel/filesystem behavior and has not been verified on the target OS build.
The query uses the macOS libSystem `fgetattrlist` entry point through cgo;
Darwin builds without cgo refuse ACL verification and therefore fail closed.

Inert temporary-file fixtures passed on M5 on 2026-11-03 under the agent's own
released CPU build admission `20261103T024034Z-m5-sirsi-software-admin-58568bda628968e6771058a6e4048b1c`;
terminal receipt `fc369a315f43463b8c18e1c006b940bc` records success. Earlier
attempts failed compilation and then ACL fixtures; those outcomes are retained.
After replacing the deprecated direct ACL syscall with the libSystem bridge,
the cgo fixture suite and the no-cgo refusal fixture passed under released M5
admission `20261103T024824Z-m5-sirsi-software-admin-d7ddefc9e71a9c1fa76d0e8a01cb9e48`;
terminal receipt `aa12596637064ffbb0b88fd7358e9fe0` records success.
The full package fixtures subsequently passed both with cgo and without cgo
under released M5 admission `20261103T024925Z-m5-sirsi-software-admin-bb95dbd84a280f770b089450545377f5`,
terminal `f9d58c1837a9406db84689dc09526788`. Without cgo, the two fixtures
requiring actual ACL reads are skipped, and the refusal fixture remains active.
The revised source received a bounded read-only review, not live qualification.
Fixture coverage does not
prove signature verification, account resolution, crash consistency, ACLs,
root custody, production behavior, or successful rollback on a real target.
