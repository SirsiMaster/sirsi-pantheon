package guard

import "github.com/SirsiMaster/sirsi-pantheon/internal/platform"

// processDisplayName presents the native engine as the owned SNE service.
// Python receives no special alias: if one appears, Horus says Python plainly.
func processDisplayName(_ platform.Platform, proc ProcessInfo) string {
	if proc.Name == "sirsi-inference" || proc.Name == "sirsi-infer" {
		return "SNE local inference"
	}
	return proc.Name
}
