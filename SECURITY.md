# Security Policy

## Supported Versions

This project is in active pre-1.0 development. Only the latest release on the `main` branch receives security updates.

| Version | Supported |
| ------- | --------- |
| latest `main` | Yes |
| prior releases | No |

## Reporting a Vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Report suspected vulnerabilities privately via GitHub's [private vulnerability reporting](https://github.com/cacack/my-family/security/advisories/new) form.

Include, where possible:

- A description of the issue and its impact
- Steps to reproduce (proof-of-concept code, affected endpoints, required preconditions)
- The commit SHA or release version you tested against
- Any suggested mitigation

You can expect an initial acknowledgement within 7 days. Because this is a solo-maintained project, remediation timelines are best-effort and depend on severity.

## Scope

In scope:

- The `myfamily` server binary and its HTTP API
- The embedded Svelte frontend
- GEDCOM import/export handling
- Database projection and event-store code

Out of scope:

- Self-hosted deployments misconfigured by the operator (e.g. exposing the server to the public internet without a reverse proxy, auth, or TLS)
- Denial of service from deliberately oversized or malformed GEDCOM files (the import path is trusted-input by design)
- Issues in third-party dependencies that already have a published advisory — please report those upstream
