# SNE Atomic Catalog Update, Rollback, and Removal Gate

Date: 2026-08-18

## Result

Pantheon now stores signed runtime catalogs as immutable SHA-256-named bundles
and selects one through an atomically replaced `runtime-catalog-current`
symlink. A real M5 transaction probe passed:

1. Original current version:
   `bbcdbaf9f86b75e58768c7c567f0f5389b4aad88d601e5e5bfc78998df70a35d`.
2. Copied catalog-ID-only probe was signed and installed as
   `6beef3da74bafbebb3af48dd924ef3da8cb2d6ac84edef1d2cb3112cb7b4ceaa`.
3. Pantheon's default signed loader observed catalog ID
   `sne-gemma4-v1-transaction-probe` with 11 entries.
4. Atomic rollback restored the original version.
5. Pantheon's loader observed `sne-gemma4-v1` with 11 entries.
6. Inactive probe removal succeeded.
7. Current version remained the original SHA-256.

The transaction suite also proves that an invalid signature or mutated catalog
does not change current, active-version removal is rejected, rollback rechecks
signature and materialization, and a version containing unexpected files cannot
be removed by the narrow remover.

## Claim boundary

This proves M5 catalog control-plane transactions. It does not prove model
package upgrade, clean-host installation, M1 behavior, or serving performance.
