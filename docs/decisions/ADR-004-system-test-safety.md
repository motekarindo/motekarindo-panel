# ADR-004: System Test Safety

## Status

Accepted

## Date

2026-06-19

## Context

Motekar Panel manages real server resources: packages, users, groups, filesystems, quotas, firewalls, web servers, PHP-FPM pools, databases, mail services, DNS, systemd units, and certificates.

Tests for these modules can damage a developer workstation or operator server if they run directly on the host machine. Installing packages, editing `/etc`, restarting services, changing firewall rules, or creating Linux users must not happen accidentally.

## Decision

Do not run destructive or privileged OS-module tests directly on the host machine.

Host machine tests are limited to:

- Unit tests.
- Golden file tests.
- Static validation.
- Build and lint commands.
- Non-privileged API and handler tests.

The following test actions must run only inside disposable environments:

- Package installation or removal.
- Writes to `/etc`, `/var`, `/usr`, `/home`, service config paths, or systemd directories.
- Service reloads or restarts.
- Firewall changes.
- SELinux changes.
- User/group creation.
- Quota configuration.
- PHP-FPM, Nginx, Apache, database, DNS, or mail service manipulation.

System tests must require an explicit safety marker such as:

```bash
MOTEKAR_TEST_ALLOW_SYSTEM=1
```

Without the marker, system tests must refuse to run.

## Approved System Test Environments

- Disposable Ubuntu 24.04 VM.
- Disposable LXD container when the target behavior is compatible with containers.
- Disposable Multipass instance.
- Dedicated throwaway test VPS.
- CI runner created specifically for destructive system tests.

The default developer host is not an approved system test environment.

## Consequences

- Adapter development must start with unit tests, golden config tests, and dry-run execution plans.
- OS integration tests need environment provisioning scripts before dangerous modules are implemented.
- Every destructive test suite must have preflight checks.
- Every destructive test suite must log what files, packages, services, and users it touches.
- System tests may be slower, but they will be safer and more trustworthy.

## Alternatives Considered

### Run Tests Directly on Developer Host

Pros:

- Faster feedback.
- Simpler setup.

Cons:

- Can break local Nginx/Apache/PHP/database/mail/firewall configuration.
- Can create users, permissions, or packages that are hard to clean up.
- Too risky for a server control panel.

Rejected.

### Only Use Mock Tests Forever

Pros:

- Safe and fast.

Cons:

- Cannot prove package installation, systemd behavior, service reloads, firewall behavior, quotas, or real runtime integration.
- Would produce a control panel that looks correct but fails on real servers.

Rejected.

