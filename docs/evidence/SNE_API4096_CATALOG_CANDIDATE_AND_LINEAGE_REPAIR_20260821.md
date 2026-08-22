# SNE API4096 Catalog Candidate and Lineage Repair

**Date:** 2026-08-21  
**Classification:** Commercial-product release governance  
**Claim scope:** Catalog construction, identity, and lineage only. No model, Metal, performance, signing-upload, installation, or activation claim.

**Owner-readable mirrors:** [Native Google Doc](https://docs.google.com/document/d/1Mw9-3FLrMJQwN9DKF2Egeq53SGHz39aMPZHv3Faj9RQ/edit) and `Desktop/Sirsi - Owner Reading Room/SNE/SNE_API4096_CATALOG_CANDIDATE_AND_LINEAGE_REPAIR_20260821.{md,html}`.

## Result

Pantheon now has a purpose-built `sirsi-sne-catalog-candidate` command for the post-admission API4096 bridge. It accepts only an already signed current catalog plus an accepted API4096 v2 receipt, the promoted product-candidate pointer, the package's API4096 parent record, an explicit runtime identity, and an exclusive output path.

The command derives the service, native runtime, MLX, metallib, JACCL, and model-manifest identities from the package's actual bytes. It requires the package, receipt, ancestry, and promotion pointer to agree; requires every API4096 correctness, realistic-use, memory, quality, and wiring gate; verifies the package dependency boundary; and emits an unsigned successor catalog. It has no private-key, signing, install, activation, or catalog-overwrite capability.

Pantheon's macOS CI explicitly builds and runs the negative admission contract. The command remains release-governance tooling and is intentionally excluded from the public GoReleaser archive.

## Regression discovered

During current-state negative testing, the repository's `sne-gemma4-v2` catalog failed detached-signature verification. Investigation proved two pre-existing staging replacements:

- repository catalog: 11-entry direct-paged/MTP staging composition, SHA-256 `36073d1a7f518075fccca5c8b37c8b49903056a6824bb9a5a7a141aa14fb6291`;
- repository admission registry: 16-entry staging composition without the admitted v26 tuple;
- canonical evidence and the immutable local catalog store required the 12-entry v26 catalog, SHA-256 `aca14182c98fdab493491322d14d49ed84a6da861b718a03164bffe513101019`.

The durable mode-0600 catalog private key derived the pinned repository public key exactly. The immutable catalog store contained the admitted v26 catalog and detached signature SHA-256 `ed649debfaf22259953aec5f7ff87e3f35c32c93103e21d51559b722e9692c94`, and that pair verified successfully. The live durable model-admission registry contained the required unique `12b-affine8-paged-v26` tuple.

The exact immutable catalog/signature and live admitted registry were restored to the repository. Both accidental staging versions and the temporary replacement signature were retained with explicit `superseded-20260821-*` names. No historical evidence was erased.

## Executable prevention

`scripts/test-sne-api4096-catalog-candidate-admission.zsh` verifies the real signed current catalog, supplies no admission evidence, requires exit 67, requires no output creation, and proves the authoritative catalog remains byte-identical. The generator's Go test changes one accepted gate to rejected and proves that the entry cannot be produced.

## Evidence

```text
go test ./cmd/sirsi-sne-catalog-candidate ./internal/sne -count=1
ok github.com/SirsiMaster/sirsi-pantheon/cmd/sirsi-sne-catalog-candidate
ok github.com/SirsiMaster/sirsi-pantheon/internal/sne

sne_signed_catalog_lineage accepted=true predecessor_immutable=true active_tuple_unique=true v26_present=true
sne_api4096_catalog_candidate_admission accepted=true missing_admission_exit=67 output_created=false authoritative_catalog_unchanged=true signing_capability=false install_capability=false activation_capability=false
```

## Remaining boundary

No API4096 successor catalog has been generated because no accepted live API4096 receipt exists yet. After the M5 graphical Metal session admits the exact parent, the workflow is: create the bound product parent, promote the copied candidate, run this generator, independently review the unsigned diff, then separately authorize signing and catalog installation. Performance qualification and public claims remain independent gates.
