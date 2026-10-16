# Observer and expensive-job launch supervision

Classified as internal tooling. Add a registered-session Ma’at supervisor with bounded process-group cleanup and strict live reservation parsing. Add a private observer launcher that refuses unsigned or unapproved replacement bytes before listener startup, without opening an authorization dialog. The observer's runtime code and existing private endpoints are unchanged.

The cooperative hooks and launcher are not a security sandbox against deliberate same-user tampering, arbitrary direct OS execution or supervisor SIGKILL. Deployment evidence and lane-specific acceptance remain separate from source tests.
