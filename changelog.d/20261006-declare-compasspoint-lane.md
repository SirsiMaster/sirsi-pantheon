### Fixed

- **`claude-m5-compasspoint` is a declared router lane.** It could send but nothing could be sent to it: the router refused every item addressed to it as an undeclared identity, so SSA and Ra could not reply. It now has a registry entry (attended session, no auto-wake).
