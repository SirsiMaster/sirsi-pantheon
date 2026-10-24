### Provider readiness

- Validate the bounded JSON completion envelope returned by the existing one-token readiness probe. HTTP 200 alone no longer marks a provider ready; malformed, error, empty, unterminated, trailing-data, oversized, cancelled, or failed completion responses remain unavailable. Valid one-token `length` and reasoning-only terminal responses are accepted.
- Add inert HTTP fixtures that exercise served-model discovery and the real readiness POST together. These fixtures are authored evidence and remain unqualified until the repository owner runs them.
