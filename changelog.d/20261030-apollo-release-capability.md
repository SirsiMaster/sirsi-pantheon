fix(core): bind Apollo capability in package inventory

Add independent enabled/disabled Apollo capability expectations to the Go
package inventory and signed Info.plist contract. Enabled requires exact
profile bytes; disabled forbids a profile. Development retains its legacy
inventory mode while release wiring and runtime resolution remain in progress.

Refs: docs/evidence/APOLLO_NATIVE_PACKAGE_COMPOSITION_20261015.md, contracts/stacklab/pantheon-release-artifact-recipe-v1.json
