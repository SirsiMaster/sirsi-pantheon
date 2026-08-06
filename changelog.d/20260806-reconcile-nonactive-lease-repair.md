Unreleased — `ReconcileOperationalState` now clears impossible ownership on non-active task rows before the expiry pass, so `router doctor --fix` recovers poison without direct SQLite edits.
