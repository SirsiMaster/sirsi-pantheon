# Release consumer preservation

Component: Ra release lifecycle, under existing ADR-061 durable-delivery and native controller image-adoption controls.

An installation updates pathnames; it does not prove an already running controller loaded those bytes. Conversely, forcibly restarting all controllers can interrupt their active consumers. `scripts/release-train.sh` must neither signal every wake loop nor override Ma'at windows to publish. Existing controllers observe the installed image and yield at their safe consumer boundary. A lifecycle repair for a genuinely stuck idle controller requires its exact native generation and owned-controller evidence; this recipe is not a blanket restart authorization.

After installation, independently read kernel image identity and generation, actual consumer continuity, canonical queue results and cleanup on each enrolled host, including Studio where applicable. Busy consumers remain preserved and the rollout stays honestly incomplete until adoption is proven. The release script reports installation only, not a full fleet acceptance.

The source guard rejects the former forced-restart and automatic-override patterns and is run in hosted CI. It does not start services, install a package or simulate a successful live adoption. Existing M1/M5/Studio actual official121 receipts remain separate evidence.

Canonical repository source is authority. Owner Reading Room and native Workspace copies for this unreleased correction remain pending; do not claim three-home publication completed.
