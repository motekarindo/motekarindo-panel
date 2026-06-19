# ADR-001: Initial Product Architecture for Motekar Panel

## Status

Accepted

## Date

2026-06-19

## Context

Motekar Panel is a full server control panel for Linux hosting operations. It should compete in the same product category as cPanel, Plesk, Webuzo, aaPanel, and similar systems, while remaining lightweight enough for small VPS deployments.

The product target includes servers as small as 1 vCPU and 2 GB RAM. This makes runtime footprint, number of required resident services, and operational simplicity first-order architecture concerns.

The product is not scoped as an MVP. Email, reseller concepts, billing/license readiness, DNS, database, web hosting, SSL, backup, and security controls are part of the full product direction. Delivery can still be staged, but the architecture must not block those capabilities.

## Decision

Use Go for the main panel web/API application.

Use Go for the privileged local agent.

Use a custom operational dashboard tailored to hosting workflows instead of a generic admin framework.

Use PostgreSQL as the default and minimum supported panel database.

Use PostgreSQL-backed jobs as the default queue so Redis is not required on minimal installations.

Include mail hosting in the product scope and first public release plan, provided the implementation satisfies security and reliability requirements.

Do not include container-based app hosting in the product direction. Motekar Panel should manage native OS services and runtimes.

Start as an open source project with a possible commercial path later.

Design license, billing, and reseller concepts from the beginning so future provider and commercial workflows are not blocked.

## Alternatives Considered

### Laravel with Filament

Pros:

- Fast admin UI development.
- Mature auth, queues, policies, forms, tables, and ecosystem.
- Familiar to many web developers.

Cons:

- Higher runtime footprint than a small Go service.
- Requires PHP application runtime for the panel itself.
- Filament is optimized for CRUD/admin workflows, while Motekar Panel needs a specialized operational UI with jobs, service health, config previews, logs, and privileged action flows.

Rejected as the default because the product explicitly targets low-spec VPS deployments.

### Node/TypeScript

Pros:

- Strong frontend ecosystem.
- Good developer velocity.
- Easy full-stack sharing of types.

Cons:

- Higher dependency surface.
- More moving parts for packaging and long-term system service operation.
- Less ideal than Go for a privileged system agent.

Rejected as the default, but still acceptable for specific tooling if needed later.

### Rust Agent

Pros:

- Strong safety guarantees.
- Excellent performance.
- Good for low-level system components.

Cons:

- Higher implementation complexity.
- Slower iteration for the expected team profile.
- Go is sufficient for the agent's needs and easier to distribute and maintain.

Rejected in favor of Go.

### SQLite for Minimal Installs

Pros:

- Very lightweight.
- No separate database service.

Cons:

- Less suitable for job state, audit logs, concurrent operations, and future multi-node control plane behavior.
- Would create an extra migration/support path away from the intended production database.

Rejected. PostgreSQL is the minimum supported panel database.

### Container-Based Hosting

Pros:

- Strong isolation.
- Easier app packaging for some workloads.

Cons:

- Adds a major runtime dependency and operational model.
- Conflicts with the initial direction of native OS service management.
- Increases support burden across small VPS environments.

Rejected for the product direction unless a future ADR changes this decision.

## Consequences

- Motekar Panel should be structured as Go binaries and packages.
- The default install should require PostgreSQL but not Redis.
- The UI needs more deliberate product design than a generic admin framework would provide.
- Feature implementation must preserve low memory usage and avoid unnecessary resident services.
- Mail hosting must be designed with the same seriousness as web/database/DNS, not treated as an afterthought.
- Commercial/provider concepts should appear in the data and permission model early, even if billing/license execution comes later.

## Follow-Up Work

- Define the Go project scaffold.
- Define the PostgreSQL schema and migration tooling.
- Define the HTTP API contract and error model.
- Define the agent action registry and local communication protocol.
- Define the first UI information architecture.
- Define minimum and recommended server requirement checks in the installer.
