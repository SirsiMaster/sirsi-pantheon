### Fixed
- Canonicalize owned Ma’at test fixture roots on macOS before using them as HOME. Production no-follow path checks remain unchanged. A deterministic aliased-TMPDIR regression and the original failed cases pass in normal and race modes; restoring the old helper fails the regression.
