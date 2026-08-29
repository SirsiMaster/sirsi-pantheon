//go:build darwin

package platform

// CheckDiskAccess is deliberately and completely non-invasive. macOS exposes no
// public API that answers whether Full Disk Access is granted. Touching TCC.db,
// Desktop, Documents, Downloads, Mail, Messages, Safari, or another protected
// resource to infer the answer can itself register access or display consent UI.
// Resident Pantheon therefore reports "not verified" and leaves verification to
// the explicit operation that actually needs the protected resource.
func CheckDiskAccess() DiskAccess {
	return DiskAccess{
		Level:   AccessNone,
		Blocked: []string{"not verified without an explicit protected-resource action"},
	}
}

// HasFullDiskAccess is the convenience boolean (AccessFull) for callers that only
// care whether Sirsi can see everything.
func HasFullDiskAccess() bool { return CheckDiskAccess().Level == AccessFull }
