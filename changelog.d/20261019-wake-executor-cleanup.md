### Fixed

- Keep bounded Ma'at executor cleanup distinct from resident wake-observer protection. An inherited launchagent wake mechanism alone no longer makes a successful short-lived executor uncloseable; live resident observers with published lane state remain protected.
- Canonicalize installed executable aliases when creating wake service definitions so the no-follow runtime upgrade watcher observes the real installed application rather than a Homebrew symlink.

The M5 official119 runtime rollout was independently checked against loaded kernel device/inode identity for all17 wake controllers; two sequential canonical queue requests were completed by the same actual119 worker. Two busy M1 consumers remain intentionally preserved pending their idle boundaries; this does not claim whole-fabric qualification.
