package main

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/completionproof/entryfixture"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Invoke the actual shipped command entry, not Enforce directly. A refusal
// must leave the durable item entirely unchanged; a completed proof must close
// it and record the supplied result. No test is parallel: Cobra globals/cwd
// and fixture environment are scoped and restored synchronously.
func TestRouterCloseCompletionAuthorization(t *testing.T) {
	for _, tc := range entryfixture.Cases() {
		t.Run(tc.Name, func(t *testing.T) {
			f := entryfixture.New(t, tc)
			before, err := f.Store.Get(f.ID)
			if err != nil {
				t.Fatal(err)
			}
			flags := routerCloseCmd.Flags()
			type savedFlag struct {
				name, value string
				changed     bool
			}
			var saved []savedFlag
			for _, name := range []string{"agent", "proof", "result", "ack", "blocked"} {
				flag := flags.Lookup(name)
				saved = append(saved, savedFlag{name, flag.Value.String(), flag.Changed})
			}
			previousArgs := append([]string(nil), flags.Args()...)
			t.Cleanup(func() {
				for _, prior := range saved {
					flag := flags.Lookup(prior.name)
					if err := flag.Value.Set(prior.value); err != nil {
						t.Errorf("restore --%s: %v", prior.name, err)
					}
					flag.Changed = prior.changed
				}
				// Restore positional arguments without interpreting prior args as
				// flags or modifying any of the restored bound flag values.
				if err := flags.Parse(append([]string{"--"}, previousArgs...)); err != nil {
					t.Errorf("restore positional arguments: %v", err)
				}
			})
			const resultBody = "entry fixture completed"
			if err := routerCloseCmd.ParseFlags([]string{
				"--agent", entryfixture.Agent, "--proof", f.Proof, "--result", resultBody,
				"--ack=false", "--blocked=false", f.ID,
			}); err != nil {
				t.Fatalf("actual close flag parsing: %v", err)
			}
			if closeAgent != entryfixture.Agent || closeProof != f.Proof || closeResult != resultBody || closeAck || closeBlocked {
				t.Fatal("actual close flags did not bind the requested completion inputs")
			}
			args := flags.Args()
			if err := routerCloseCmd.ValidateArgs(args); err != nil {
				t.Fatalf("actual close positional arguments: %v", err)
			}
			err = routerCloseCmd.RunE(routerCloseCmd, args)
			if tc.Refusal != "" {
				if err == nil || !strings.Contains(err.Error(), tc.Refusal) {
					t.Fatalf("want completion refusal %q, got %v", tc.Refusal, err)
				}
				f.AssertUnchanged(t, before)
				f.AssertValidator(t)
				return
			}
			if err != nil {
				t.Fatalf("completed proof refused: %v", err)
			}
			after, err := f.Store.Get(f.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Status != routerstore.StatusClosed || after.Result != resultBody || after.Closed == "" {
				t.Fatalf("actual close did not commit result: %+v", after)
			}
			f.AssertValidator(t)
		})
	}
}
