# ADR-002: Product Name and Web Server Policy

## Status

Accepted

## Date

2026-06-19

## Context

The original working name `openpanel` conflicts with an existing hosting control panel product. The project needs a distinct working product name before implementation and public-facing documentation progress further.

The product is also intended for shared hosting. In shared hosting, the web server is foundational infrastructure: it affects vhost generation, SSL, rewrite rules, redirects, reverse proxy behavior, logs, application runtime integration, and support expectations.

Allowing each user, domain, or subdomain to choose a different web server increases operational risk and support complexity, especially because Motekar Panel is intentionally native-OS based rather than container-based.

## Decision

Use `Motekar Panel` as the working product name.

The product is owned by Motekar Teknologi Indonesia.

The primary web server is selected once during installation.

The selected web server cannot be changed from the panel after installation.

All hosting accounts, domains, and subdomains on that server inherit the selected web server.

The panel must not provide:

- Per-user web server selection.
- Per-domain web server selection.
- Per-subdomain web server selection.
- An in-panel primary web server switcher.

Changing the primary web server requires reinstalling the server or using a future offline migration runbook that explicitly documents risks and compatibility limits.

## Consequences

- Installer UX must ask for the primary web server early.
- The selected web server must be stored as an immutable server-level setting.
- Service adapters can support multiple web servers across different installations, but not multiple primary web servers inside one installation.
- Runtime selection remains per project/site: PHP, Node.js, Python, static, or reverse proxy.
- PHP versions may vary per site through PHP-FPM pool configuration.
- Node.js and Python versions may vary per site where the installed runtime manager supports it.
- The product becomes simpler and safer to operate on low-resource VPS instances.

## Alternatives Considered

### Web Server Per User or Domain

Pros:

- More flexibility for advanced users.
- Similar to some containerized or multi-engine control panels.

Cons:

- High operational complexity.
- Higher chance of broken SSL, rewrite, proxy, and log behavior.
- Harder support model for shared hosting providers.
- Less aligned with a lightweight native OS architecture.

Rejected.

### In-Panel Web Server Switcher

Pros:

- Convenient for admins.

Cons:

- Risky after websites and custom configs exist.
- Requires complex migration logic and rollback handling.
- A failed switch could break all hosted websites.

Rejected for normal product UX. A future offline migration runbook may be considered separately.

