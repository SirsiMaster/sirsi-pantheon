**Developer ID signing identity resolution** (2026-09-28). The macOS release
job now derives the application signing identity from the certificate actually
imported into its temporary keychain, preventing a stale secret label from
breaking an otherwise valid signed/notarized package build.
