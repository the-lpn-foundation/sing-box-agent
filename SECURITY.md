# Security Policy

## Supported Versions

Only the latest published minor release receives security fixes. Older
versions may be upgraded by re-tagging and rebuilding from `main`.

| Version   | Supported |
|-----------|-----------|
| latest    | ✅        |
| < latest  | ❌        |

## Reporting a Vulnerability

**Please do not open public GitHub issues for security problems.**

Report privately by email to the maintainer. Include:

- A description of the issue and potential impact.
- Steps to reproduce (minimal proof-of-concept preferred).
- Affected version / commit SHA.
- Any suggested mitigation.

You can expect:

- Acknowledgement within **72 hours**.
- An initial assessment within **7 days**.
- A fix or mitigation plan for confirmed critical issues within **30 days**.

Once a fix is released, a CVE / advisory will be published through GitHub
Security Advisories and credited to the reporter (unless anonymity is
requested).

## Scope

In scope:

- The agent REST API and authentication / signing logic.
- The desired-state reconciliation engine.
- Build and release artifacts produced by this repository's workflows.

Out of scope (please report upstream):

- Vulnerabilities in [sing-box](https://github.com/SagerNet/sing-box) itself.
- Vulnerabilities in third-party Go dependencies — these are tracked via
  [`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) in
  CI; please also notify the upstream project.

## Hardening Recommendations for Operators

- Always run the agent behind TLS in production. Generate `token` and
  `secret` with `openssl rand -hex 32` — never reuse values across servers.
- Restrict inbound network access to the agent's API port
  (default `8080`) to trusted management hosts only.
- Run the agent as an unprivileged user where possible; grant only the
  file permissions required to read/write the managed sing-box config.
- Rotate `token` and `secret` periodically, and immediately if a host is
  compromised.
- `/metrics` is served **unauthenticated** on both the metrics port and the
  API port. If your infrastructure is sensitive, restrict access with a
  firewall or bind the ports to loopback / management interfaces only.
- The HMAC signature covers the request path only — it does **not** cover
  the query string. Do not place sensitive or security-relevant parameters
  in GET query strings and do not rely on the signature to protect them.
- Generate `secret` the same way as `token`: a random value of at least 32
  characters (e.g. `openssl rand -hex 32`). Never reuse values across
  servers.
- The agent writes the sing-box config atomically with `0600` permissions.
  Run the agent as an unprivileged user and keep restrictive umask / file
  permissions so the managed config is readable only by the agent and
  sing-box.
- Monitor Prometheus metrics and agent logs for unusual activity.
