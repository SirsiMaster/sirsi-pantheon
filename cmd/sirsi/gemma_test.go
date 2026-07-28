package main

import (
	"strings"
	"testing"
)

func TestRenderGemmaPromptAddsSirsiIdentity(t *testing.T) {
	p := renderGemmaPrompt("Are you the local implementation of Sirsi?")
	for _, want := range []string{"Ask Sirsi", "Sirsi Pantheon", "Cylton Collymore", "Claude Home", "Task:\nAre you the local implementation of Sirsi?"} {
		if !strings.Contains(p, want) {
			t.Errorf("gemma prompt missing %q:\n%s", want, p)
		}
	}
}
