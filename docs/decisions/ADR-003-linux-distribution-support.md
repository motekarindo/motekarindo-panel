# ADR-003: Linux Distribution Support Strategy

## Status

Accepted

## Date

2026-06-19

## Context

Motekar Panel targets shared hosting servers and needs to operate close to the OS: package installation, systemd services, web server configuration, PHP runtimes, DNS, mail, firewall, quotas, cgroups, SSL, backups, and logs.

Debian/Ubuntu and RHEL-compatible distributions differ in package managers, package names, repository availability, service defaults, filesystem paths, firewall defaults, SELinux behavior, and PHP multi-version packaging.

The product should support Rocky Linux and AlmaLinux because many hosting providers use RHEL-compatible systems, but support must be explicit and tested rather than assumed.

## Decision

Use Ubuntu 24.04 LTS as the first supported installation target.

Plan Debian 12 support after Ubuntu 24.04 LTS support is stable.

Plan support for RHEL-compatible distributions, including Rocky Linux and AlmaLinux.

Do not mark Rocky Linux, AlmaLinux, or other RHEL-compatible systems as production-supported until the following pass system tests:

- Installer.
- Package manager adapter.
- Web server adapter.
- PHP runtime adapter.
- Database adapter.
- DNS adapter.
- Mail adapter.
- Firewall adapter.
- SELinux behavior.
- SSL issuance and renewal.
- Backup and restore.
- Upgrade and rollback.

## Consequences

- OS-specific behavior must live behind adapter boundaries.
- The installer must detect OS family and version before making changes.
- Documentation must clearly distinguish supported, planned, and experimental distributions.
- The first installer may reject non-Ubuntu 24.04 LTS systems with a clear error.
- RHEL-compatible support requires dedicated CI/system test environments.
- firewalld and SELinux support are required for stable RHEL-family support.
- UFW assumptions must not leak into core firewall interfaces.

## Alternatives Considered

### Support Every Linux Distribution From Day One

Pros:

- Broader market reach.

Cons:

- High support burden.
- More untested combinations.
- Higher chance of broken hosting features.

Rejected.

### Debian/Ubuntu Only Forever

Pros:

- Simpler implementation.
- Easier test matrix.

Cons:

- Excludes common hosting-provider operating systems.
- Reduces adoption by users standardized on Rocky Linux, AlmaLinux, or RHEL-compatible stacks.

Rejected as a long-term strategy.
