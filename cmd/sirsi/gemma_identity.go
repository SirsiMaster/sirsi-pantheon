package main

import "github.com/SirsiMaster/sirsi-pantheon/internal/localai"

func renderGemmaPrompt(prompt string) string {
	return localai.RenderPrompt(prompt)
}
