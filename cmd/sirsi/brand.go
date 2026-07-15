package main

import (
	"fmt"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brand"
)

var brandFormat string

// brandCmd surfaces the ONE Pantheon palette (internal/brand) and emits it in
// the form each surface needs — so the menubar, the web dashboard, and Nexus
// derive their colors from the same source the CLI does, never a hand-copied
// hex (ADR-038, the completion-proof: the identity ships as a lever).
var brandCmd = &cobra.Command{
	Use:   "brand",
	Short: "𓂀 Pantheon brand tokens — emerald + gold, one palette for every viewport",
	Long: `Show or emit the canonical Pantheon color tokens.

  sirsi brand                     Swatch table (both schemes)
  sirsi brand tokens --format css     CSS custom properties  (dashboard, Nexus, Artifacts)
  sirsi brand tokens --format swift   SwiftUI Color extension (menubar, Swift app)
  sirsi brand tokens --format json    Stable JSON            (any tool)

Single source: internal/brand/brand.go. Green is Sirsi; green + gold are Pantheon.`,
	RunE: func(cmd *cobra.Command, args []string) error { return runBrandSwatch() },
}

var brandTokensCmd = &cobra.Command{
	Use:   "tokens",
	Short: "Emit the palette as css | swift | json for a non-Go surface",
	RunE: func(cmd *cobra.Command, args []string) error {
		switch brandFormat {
		case "css":
			fmt.Print(":root {\n" + indent(brand.CSSVars(brand.Light)) + "}\n")
			fmt.Print("@media (prefers-color-scheme: dark) {\n  :root {\n" + indent2(brand.CSSVars(brand.Dark)) + "  }\n}\n")
			return nil
		case "swift":
			fmt.Print(brand.SwiftColors())
			return nil
		case "json":
			fmt.Print(brand.JSON())
			return nil
		default:
			return fmt.Errorf("unknown --format %q (want css | swift | json)", brandFormat)
		}
	},
}

func indent(s string) string  { return prefixLines(s, "  ") }
func indent2(s string) string { return prefixLines(s, "    ") }

func prefixLines(s, pad string) string {
	out := ""
	for _, ln := range splitNonEmpty(s) {
		out += pad + ln + "\n"
	}
	return out
}

func splitNonEmpty(s string) []string {
	var lines []string
	cur := ""
	for _, c := range s {
		if c == '\n' {
			if cur != "" {
				lines = append(lines, cur)
			}
			cur = ""
			continue
		}
		cur += string(c)
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// runBrandSwatch prints a labeled swatch of every role in the dark scheme (the
// terminal ground), proving the palette resolves.
func runBrandSwatch() error {
	fmt.Println("𓂀 Pantheon brand — emerald + gold  (green is Sirsi; green + gold are Pantheon)")
	fmt.Println()
	for _, r := range brand.Roles() {
		hexDark := brand.For(brand.Dark).Hex(r)
		hexLight := brand.For(brand.Light).Hex(r)
		sw := lipgloss.NewStyle().Background(lipgloss.Color(hexDark)).Render("      ")
		name := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(hexDark)).Render(pad(r.Name(), 9))
		fmt.Printf("  %s  %s  dark %s   light %s\n", sw, name, hexDark, hexLight)
	}
	fmt.Println()
	fmt.Println("  Emit for a surface:  sirsi brand tokens --format css|swift|json")
	return nil
}

func pad(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}

func init() {
	brandTokensCmd.Flags().StringVar(&brandFormat, "format", "css", "output format: css | swift | json")
	brandCmd.AddCommand(brandTokensCmd)
}
