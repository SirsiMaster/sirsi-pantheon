- fix(supervision): `supervision.Escalates()` had no caller — lanes no wake could
  reach were classified, painted red, and never reported to anyone. Adds
  `supervision.Escalations` + `router.RouteLaneEscalations`, wired into the
  Horus supervisor sweep, deduped by stable title against the owner's open inbox.
- fix(dashboard): `fleet.go` hardcoded `Routable: true`, making `UNROUTABLE`
  unreachable. Routability is now read from the registry: 10 lanes that were
  rendering as merely idle are unroutable, 6 of them holding open work.
- fix(routerstore): recognize schema v8–v14, already deployed to the live store
  by an out-of-band build. All additive; ports the definitions so binaries stop
  refusing a store they can safely read.
