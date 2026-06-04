# Security Policy

## Supported versions

Hitch is pre-1.0 software. Security fixes are supported on the latest commit on `main` and on the latest tagged release when releases are available.

## Reporting a vulnerability

Please do not open a public issue for a suspected vulnerability.

Report security issues through GitHub private vulnerability reporting for this repository. If that is unavailable, contact the repository owner directly and include:

- affected Hitch version or commit
- affected command, API endpoint, adapter, or installer path
- reproduction steps
- expected impact
- whether any secrets, local files, or audit records were exposed

We will acknowledge valid reports, investigate, and publish fixes with release notes when appropriate.

## Security boundaries

Hitch is a local developer tool. It is intended to bind to loopback by default and to execute only handlers that the local user configured. Treat handler commands and captured native hook payloads as sensitive local data.
