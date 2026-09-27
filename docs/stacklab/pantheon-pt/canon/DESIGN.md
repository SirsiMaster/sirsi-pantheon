# Design — Pantheon PT

Source authority flows from the Go engine to product surfaces, then to deterministic package and cask renderers. Release evidence is a composed DAG: source -> tests/build -> unsigned package -> retained release inputs -> canonical cask bytes -> signing/notary -> remote release -> installed lifecycle. A missing node leaves the release state open.
