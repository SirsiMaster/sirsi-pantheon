# Security and threat model — Pantheon PT

Threats include sibling-engine drift, PATH substitution, release-input replacement, malformed cask records, credential leakage, unsigned artifacts presented as release-ready, and destructive lifecycle actions. Controls are canonical-engine identity, descriptor/byte-bound release inputs, no-follow/create-only publication, exact hash readback, explicit development/commercial modes with distinct names, mandatory Developer ID plus notarization/stapling for commercial artifacts, rollback receipts, and fail-closed lifecycle validation.
