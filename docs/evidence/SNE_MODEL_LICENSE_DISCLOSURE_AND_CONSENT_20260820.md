# SNE Model License Disclosure and Consent

**Date:** 2026-08-20  
**Status:** Source and focused dashboard verification passed  
**Classification:** Commercial-product licensing closure; live installation pending

## Defect

Pantheon required `accept_license=true` before model acquisition and recorded acceptance in the transactional checkout, but the model-management UI asked the user to accept generic “manifest-bound license terms.” It did not name the exact license or provide the authoritative terms URL before consent.

That was not sufficient for a launch-grade installer.

## Repair

- Pantheon's SNE read model now exposes `license_id`, `license_url`, and `license_acceptance_required` per acquisition tuple.
- The model card shows the exact license identifier and an external terms link before installation.
- The installation confirmation names the model, license identifier, authoritative URL, and receipt consequence.
- License URLs come from a fail-closed Pantheon registry. Current supported identities are:
  - `gemma-terms` -> `https://ai.google.dev/gemma/terms`
  - `apache-2.0` -> `https://www.apache.org/licenses/LICENSE-2.0`
- A source with an unknown or missing terms URL cannot expose an enabled Install action.
- The existing server-side license-acceptance requirement and transactional receipt remain unchanged.

## Verification

Focused dashboard routing, license registry, and install tests pass. Tests prove the Gemma terms mapping, unknown-license rejection, license disclosure contract in the rendered SNE page, server-side acceptance enforcement, same-origin control, and unknown-field rejection.

No model download, package mutation, SNE process, GPU workload, memory pressure, or performance test ran.

## Claim boundary

This closes source/UI licensing semantics only. A normal-user clean-host installation, actual terms review, signed artifact acquisition, one-copy checkout, cancellation/resume, and uninstall evidence remain required before pilot release.

**Workspace mirror:** Pending connector synchronization. Repository source is authoritative; the Owner Reading Room copy is current.
