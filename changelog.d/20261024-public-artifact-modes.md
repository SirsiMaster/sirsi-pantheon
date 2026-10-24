## Correct restrictive builder umask in app/PKG staging

Normalize only generated owned bundle/payload metadata before signing/pkgbuild:
0755 directories, 0644 public resources, 0755 exact app/system CLI allowlist.
Reject symlinks, special files, hardlinks and unexpected executable locations
before any chmod. Post-signing and expanded-package checks are read-only.
Inert fixtures compare umask077/022 and independent-UID mode bits plus unchanged
source bytes. No installed app, permissions, signing service or published asset
is modified. Tests/build/BOM/install qualification remains unrun by the author.

Canonical Go inventory independently rejects private root/resource/signature
modes and set-ID/sticky/writable/executable promotion. An explicit fixed,
nonrelocatable, strict-identifier/version-checked Pantheon component selects
native BundleOverwriteAction=upgrade so old private containers are replaced,
not merged. Actual corrective upgrade remains SHA/SSA-owned qualification.
Package entry also rejects a bad signed input app read-only before any copying;
the valid source app cannot diverge from a silently repaired package-only copy.
