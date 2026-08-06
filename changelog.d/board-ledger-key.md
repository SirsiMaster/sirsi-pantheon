- fix(board): emit `ledger` alongside `board` in the router-board payload.
  index.html renders `d.ledger`; the payload carried only `d.board`, so every
  tile bound to it read undefined — the page showed nonsense while /api/ledger
  and the CLI returned correct numbers. Adds a payload contract test asserting
  every field the page reads is present, since a dropped key is invisible in Go.
