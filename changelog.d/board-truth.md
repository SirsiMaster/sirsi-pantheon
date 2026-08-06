- fix(routerstore): read-compatibility with a store newer than the binary. The
  write guard stays — a binary must never migrate a schema it does not define —
  but refusing to READ turned a coordination problem into a fleet-wide blackout.
  `OpenReadOnly` opens driver-enforced read-only, reads only the tables this
  binary defines, and carries a `SchemaGap` whose banner every surface renders.
- fix(dashboard): 8734 is now served by the SAME process and handler as 9119.
  It was a separate Python board computing its own lane states, and reported
  nine lanes WORKING while every one had zero live processes. Two producers
  cannot agree by discipline, only by being one producer.
