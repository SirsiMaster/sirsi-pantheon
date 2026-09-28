**Fail-closed signing identity guard** (2026-09-28). The macOS package job
now always verifies that the temporary signing keychain exists and resolves a
Developer ID Application identity before package production, preventing a
conditional mismatch from silently falling back to ad-hoc signing.
