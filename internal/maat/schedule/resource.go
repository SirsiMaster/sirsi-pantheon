package schedule

// resourcePool recognizes only the two historical bare Studio machine names.
// It is a comparison key, never a replacement for stored or returned identity.
// Rails, components, other names and executor host bindings remain distinct.
func resourcePool(resource string) string {
	if resource == "m5-studio" {
		return "studio"
	}
	return resource
}

func sameResource(a, b string) bool { return resourcePool(a) == resourcePool(b) }

// poolCapacity reads historical alias keys without rewriting them. If both
// aliases were configured differently, the smaller positive limit is safe.
func poolCapacity(capacities map[string]int, resource string) int {
	if resourcePool(resource) != "studio" {
		return capacities[resource]
	}
	a, b := capacities["studio"], capacities["m5-studio"]
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}
