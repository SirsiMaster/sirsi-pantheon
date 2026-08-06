package platform

import "errors"

// errTrashUnsupported is returned by EmptyTrash on platforms with no trash
// Sirsi manages. Returning an error rather than nil is deliberate: a caller
// asking to permanently delete must never be told the work succeeded by a
// platform that did nothing.
var errTrashUnsupported = errors.New("this platform has no trash Sirsi can empty")
