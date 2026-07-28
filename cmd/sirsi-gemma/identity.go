package main

import "github.com/SirsiMaster/sirsi-pantheon/internal/localai"

func applySirsiIdentity(system string) string {
	return localai.ApplyIdentity(system)
}

func renderCompletionPrompt(prompt string) string {
	return localai.RenderPrompt(prompt)
}
