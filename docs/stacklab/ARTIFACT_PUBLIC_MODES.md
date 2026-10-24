# Public artifact mode correction

The builder's private umask must not become root-owned installation policy.
Before code signing the generated Pantheon.app, `scripts/artifact_modes.py`
preflights its complete bounded owned tree without following links. It applies
0755 to directories, 0644 to public regular resources and 0755 only to
Contents/MacOS/sirsi and sirsi-menubar. Signing metadata is created with umask022;
a read-only post-signing mode check refuses private signer output.

Before pkgbuild, the same helper normalizes only the invocation-owned payload
copy, including Applications and usr/local/libexec/sirsi-pantheon ancestors and
its exact sirsi executable. The signed input app's bytes and metadata are not
changed by package assembly. Expanded payload modes are checked read-only;
SSA's actual BOM/install proof must independently verify the recorded root
ownership and public read/traverse/execute modes. Current-owner os.access is
not independent-user readability evidence.

Before creating any package staging, build-pkg checks the signed INPUT app
read-only. A bad private DMG/source app therefore cannot be silently repaired
only in the package copy. The actual-entry fixture fences before staging and
all packaging/signing tools, including when the check is deliberately omitted;
it asserts bad-input refusal and unchanged source bytes/modes, plus a valid
control reaching the inert fence. It does not invoke a real Installer or sign.

The canonical Go `package-inventory` independently rejects an app root or any
payload entry outside exact 0755-directory/0755-two-executable/0644-resource modes,
including set-ID/sticky/writable and signature-resource modes. Root mode is also
recorded separately from the existing entry list. It does not repair metadata.

Existing private133 directories require replacement, not just fresh extraction.
The single fixed component in `scripts/pantheon-pkg-components.plist` selects
Applications/Pantheon.app, nonrelocatable, strict identifier, version checked,
and BundleOverwriteAction=upgrade. Local `/usr/share/man/man1/pkgbuild.1`
documents that upgrade atomically replaces the old bundle while update preserves
old paths; neither overwrite action itself performs version checking. Existing
version checks are intentionally explicit and retained. No privileged repair
script is added. Actual upgrade over private133, rollback and ordinary-user
read/traverse/launch are still SHA/SSA qualification, NOT proved by source plist
fixtures or a correct fresh BOM alone.

Main's native metadata probe5204daa438004075a7d091f2cf7eeeae exited0:
PackageInfo SHA256ec7474e00ec3b038ba29cda56210662d31fdd08e773155edf3fbc21095ae423a.
The read-only bounded parser checks its observed direct bundle, sole upgrade and
strict-identifier memberships, empty update/atomic-update/relocate containers,
version/path/ID and overwrite-permissions=true/relocatable=false. The source XML
fixture adds only a final newline, explicitly removed for provenance comparison.
Four stderr write-permission diagnostics remain retained despite exit0; this
does not establish a real installed upgrade or clean stderr. The132 installer
skipped the higher-version133 app and restored the system CLI only; that is NOT
evidence that an actual higher-version atomic replacement preserves0700 modes.
Intentional version checking remains enabled. SHA's whole132 atomic restore was
reported successful; the actual134-over-private133 upgrade still needs proof.

The Python stdlib helper is build-time tooling only, never a payload resource or
runtime dependency. Payload inventory remains the canonical Go byte/layout and
Python-free verifier. There is no installed-app chmod, service/security change,
asset replacement or release-overlay path in this source correction. Source
fixtures use only inert owned temporary trees. Metadata preflight/descriptor
checks address ordinary cooperative staging, not hostile same-UID atomicity.

Unrun qualification: `python3 -m unittest discover -s scripts -p
'test_artifact_modes.py' -v`; shell syntax/static contracts; controlled omission
negative must fail the umask077 public-mode assertions; actual packaging/BOM,
code-signature/readback and ordinary-user installed launch belong separately to
SSA under admission. No real signing/installer test is performed by the author.
