// Command maat-advise asks the Ma'at known-failure catalog about a failure BEFORE a person
// or a model spends time on it. The pre-push gate, the CI failure workflow and the release
// train pipe a failing step's output through it: a known failure prints its cause and its
// fix at once; an unknown one says how to record it so it is known next time.
//
// It deliberately imports only the catalog package, so it still builds and runs when the
// code under test does not compile (a broken build is exactly when it is needed).
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knownfail"
)

const maxInput = 8 << 20 // keep the last 8 MiB: failures sit at the end of a long log

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Getenv("GITHUB_STEP_SUMMARY")))
}

// run returns 0 whether or not anything matched: advising must never change a gate's verdict.
func run(args []string, in io.Reader, out io.Writer, summaryPath string) int {
	step, github := "", false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--step":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "usage: maat-advise [--step NAME] [--github] < failure-output")
				return 2
			}
			i++
			step = args[i]
		case "--github":
			github = true
		default:
			fmt.Fprintln(os.Stderr, "usage: maat-advise [--step NAME] [--github] < failure-output")
			return 2
		}
	}
	b, err := io.ReadAll(io.LimitReader(in, 1<<30))
	if err != nil {
		fmt.Fprintf(out, "  𓆄 Ma'at: could not read the failure output: %v\n", err)
		return 0
	}
	if len(b) > maxInput {
		b = b[len(b)-maxInput:]
	}
	where := ""
	if step != "" {
		where = " in " + step
	}
	matches := knownfail.Match(string(b))
	if len(matches) == 0 {
		fmt.Fprintf(out, "  𓆄 Ma'at: no known failure matches this output%s.\n", where)
		fmt.Fprintln(out, "     If it can recur, record it once so it is answered instantly next time:")
		fmt.Fprintln(out, "       sirsi maat known-failures register <id> --signature '<regexp from the error>' --cause '<why it happens>'")
		if github {
			fmt.Fprintf(out, "::notice title=Ma'at: unknown failure%s::No known failure matches. Record it with sirsi maat known-failures register so the next occurrence is answered immediately.\n", where)
		}
		appendSummary(summaryPath, fmt.Sprintf("### 𓆄 Ma'at: no known failure matches%s\nIf it can recur, record it once: `sirsi maat known-failures register <id> --signature '<regexp>' --cause '<why>'`.\n", where))
		return 0
	}
	var md strings.Builder
	for _, e := range matches {
		fmt.Fprintf(out, "  𓆄 Ma'at: this is a KNOWN failure%s: %s\n", where, e.ID)
		fmt.Fprintf(out, "     what:  %s\n", e.Title)
		fmt.Fprintf(out, "     cause: %s\n", e.Cause)
		if e.Fix.Text != "" {
			fmt.Fprintf(out, "     fix:   %s\n", e.Fix.Text)
		}
		if e.Status == "resolved" {
			fmt.Fprintf(out, "     fixed in %s; guarded by %s %s\n", e.Fix.FixedIn, e.Guard.Kind, e.Guard.Ref)
		} else {
			fmt.Fprintln(out, "     status: open (recorded, no fix shipped yet)")
		}
		if github {
			fmt.Fprintf(out, "::error title=Ma'at known failure: %s::%s Fix: %s\n", e.ID, oneLine(e.Cause), oneLine(e.Fix.Text))
		}
		fmt.Fprintf(&md, "### 𓆄 Known failure%s: `%s`\n- **what:** %s\n- **cause:** %s\n- **fix:** %s\n", where, e.ID, e.Title, e.Cause, e.Fix.Text)
		if e.Status == "resolved" {
			fmt.Fprintf(&md, "- fixed in %s; guarded by %s `%s`\n", e.Fix.FixedIn, e.Guard.Kind, e.Guard.Ref)
		}
	}
	appendSummary(summaryPath, md.String())
	return 0
}

func oneLine(s string) string {
	s = strings.NewReplacer("\r", " ", "\n", " ", "%", "%25").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

func appendSummary(path, text string) {
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(text + "\n")
}
