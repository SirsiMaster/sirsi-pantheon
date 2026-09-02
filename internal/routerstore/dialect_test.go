package routerstore

import "testing"

// The rewriter is a token substitution, not a parser; these pin every rule in
// pg/README.md so a drift between rewriter and schema fails here first.
func TestRewriteForPostgres(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"placeholders", `SELECT id FROM items WHERE to_agent = ? AND status = ?;`,
			`SELECT id FROM items WHERE to_agent = $1 AND status = $2;`},
		{"placeholder inside string literal survives",
			`SELECT ? WHERE title = 'why?' AND x = ?`,
			`SELECT $1 WHERE title = 'why?' AND x = $2`},
		{"insert or ignore with semicolon",
			`INSERT OR IGNORE INTO state(key,value) VALUES(?,?);`,
			`INSERT INTO state(key,value) VALUES($1,$2) ON CONFLICT DO NOTHING;`},
		{"insert or ignore without semicolon and trailing newline",
			"INSERT OR IGNORE INTO wake_events(a) VALUES(?)\n",
			`INSERT INTO wake_events(a) VALUES($1) ON CONFLICT DO NOTHING`},
		{"strftime and randomblob",
			`INSERT INTO t(id,ts) VALUES(lower(hex(randomblob(16))),strftime('%Y-%m-%dT%H:%M:%SZ','now'))`,
			`INSERT INTO t(id,ts) VALUES(router.rand_hex32(),router.now_rfc3339())`},
		{"more than nine placeholders number correctly",
			`VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			`VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`},
		{"untouched sqlite-neutral sql",
			`UPDATE items SET status='closed' WHERE id = 'x'`,
			`UPDATE items SET status='closed' WHERE id = 'x'`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rewriteForPostgres(c.in); got != c.want {
				t.Fatalf("\n in: %q\ngot: %q\nwant: %q", c.in, got, c.want)
			}
		})
	}
}

// SQLite dialect is the identity: the rewrite seam must never alter the
// engine the store was written for (behavior-preserving, ADR-062 step 1).
func TestSQLiteDialectIsIdentity(t *testing.T) {
	in := `INSERT OR IGNORE INTO state(key,value) VALUES(?,strftime('%Y-%m-%dT%H:%M:%SZ','now'));`
	if got := sqliteDialect.rewrite(in); got != in {
		t.Fatalf("sqlite rewrite changed the statement: %q", got)
	}
}
