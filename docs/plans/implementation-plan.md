# Implementation Plan: Motekar Panel

## Overview

Motekar Panel is a full shared-hosting server control panel targeting Ubuntu 24.04 LTS first. The implementation will be delivered in ordered phases. This is not a reduced MVP scope; it is a dependency-driven build plan for the full product.

The first usable system should prove the core architecture:

- Go panel web/API binary.
- Go privileged agent binary.
- PostgreSQL panel database.
- PostgreSQL-backed job queue.
- Ubuntu 24.04 installer checks.
- Immutable web server selection at installation.
- Audited, allowlisted privileged actions.

After that foundation is stable, product features can be added as vertical slices.

## Architecture Decisions

- Product name: Motekar Panel.
- Initial OS: Ubuntu 24.04 LTS.
- Main application: Go.
- Agent: Go.
- Database: PostgreSQL.
- Queue: PostgreSQL-backed jobs.
- Web server choice: selected at installation and immutable afterward.
- First web server adapters: Nginx and Apache, but only one selected per installation.
- Container hosting: out of scope.
- Native OS service model: required.

## Phase 0: Repository Foundation

### Task 0.1: Initialize Repository Structure

**Description:** Create the initial Go monorepo layout for panel, agent, CLI, migrations, templates, docs, and tests.

**Status:** Completed in initial scaffold.

**Acceptance criteria:**

- [x] Repository has a Go module.
- [x] `cmd/motekar-panel`, `cmd/motekar-agent`, and `cmd/motekarctl` exist.
- [x] `internal/` packages exist for auth, audit, jobs, server, agent, adapters, and os detection.
- [x] `services/migrations` and `services/templates` exist.

**Verification:**

- [x] `go test ./...` passes.
- [x] `go build ./cmd/motekar-panel ./cmd/motekar-agent ./cmd/motekarctl` succeeds.

**Dependencies:** None.

**Estimated scope:** Medium.

### Task 0.2: Add Development Commands

**Description:** Add `Makefile` commands for development, testing, linting, formatting, and build.

**Status:** Completed in initial scaffold.

**Acceptance criteria:**

- [x] `make test` runs all Go tests.
- [x] `make build` builds all binaries.
- [x] `make fmt` formats Go code.
- [x] `make dev` starts the panel in development mode when dependencies exist.

**Verification:**

- [x] `make test`
- [x] `make build`

**Dependencies:** Task 0.1.

**Estimated scope:** Small.

### Task 0.3: Add Basic CI Workflow

**Description:** Add CI gates for formatting, tests, and builds.

**Status:** Completed with GitHub Actions on Ubuntu 24.04.

**Acceptance criteria:**

- [x] CI workflow runs on pull requests and pushes.
- [x] CI checks formatting.
- [x] CI runs tests.
- [x] CI builds all binaries.

**Verification:**

- [x] Local equivalent commands pass.

**Dependencies:** Task 0.2.

**Estimated scope:** Small.

## Phase 1: Core Platform

### Task 1.1: PostgreSQL Configuration and Migrations

**Description:** Add database connection loading, migration runner, and initial schema for users, sessions, roles, permissions, audit events, jobs, servers, and settings.

**Status:** Safe code foundation completed. Live PostgreSQL smoke verification is pending on a disposable database/VPS.

**Acceptance criteria:**

- [x] Panel has code path to connect to PostgreSQL through `MOTEKAR_DATABASE_URL`.
- [x] Migrations can run idempotently through `schema_migrations`.
- [x] Initial tables are defined in SQL.
- [x] Immutable server settings table can store selected web server.
- [ ] Live PostgreSQL smoke test confirms schema applies on a disposable database.

**Verification:**

- [x] Migration runner unit tests pass without touching host services.
- [x] `make test` passes.
- [x] `make build` passes.
- [ ] Migration tests pass against a disposable PostgreSQL database.
- [ ] Schema smoke test inserts and reads core records on a disposable PostgreSQL database.

**Dependencies:** Phase 0.

**Estimated scope:** Medium.

### Task 1.2: Configuration Loader

**Description:** Add typed configuration loading for panel, agent, database, logging, and environment mode.

**Status:** Completed in initial scaffold.

**Acceptance criteria:**

- [x] Missing required config fails fast with clear error.
- [x] Secrets are not logged.
- [x] Development defaults are explicit.

**Verification:**

- [x] Unit tests cover valid config, missing config, and redaction.

**Dependencies:** Phase 0.

**Estimated scope:** Small.

### Task 1.3: Structured Logging

**Description:** Add structured logging shared by panel, agent, and CLI.

**Status:** Completed in initial scaffold.

**Acceptance criteria:**

- [x] Logs include timestamp, level, component, message.
- [x] Request ID and job ID can be attached.
- [x] Secret values can be redacted.

**Verification:**

- [x] Unit tests for redaction.

**Dependencies:** Task 1.2.

**Estimated scope:** Small.

### Task 1.4: HTTP Server Skeleton

**Description:** Implement the panel HTTP server with health, readiness, version, and static asset support.

**Status:** Completed in initial scaffold.

**Acceptance criteria:**

- [x] `GET /healthz` returns healthy.
- [x] `GET /readyz` has injectable readiness checks.
- [x] `GET /version` returns build/version metadata.
- [x] Basic HTML layout renders.

**Verification:**

- [x] HTTP handler tests pass.
- [x] `motekar-panel serve` starts locally.

**Dependencies:** Tasks 1.1, 1.2, 1.3.

**Estimated scope:** Medium.

## Phase 2: Auth, RBAC, and Audit

### Task 2.1: First Admin Bootstrap

**Description:** Add CLI or installer bootstrap for creating the first owner/admin user.

**Status:** Password hashing, input validation, one-time bootstrap policy, SQL store foundation, CLI wiring, and owner role assignment completed. Audit event and live PostgreSQL verification are pending.

**Acceptance criteria:**

- [x] First admin can be created once at service policy level.
- [x] Password is hashed with a strong algorithm.
- [x] Bootstrap input validates email, display name, and minimum password length.
- [x] Bootstrap command wires service to the panel database.
- [x] Owner/admin role is assigned.
- [ ] Duplicate bootstrap is rejected.
- [ ] Audit event is recorded.

**Verification:**

- [x] Unit tests for bootstrap policy and password hashing.
- [x] Unit tests for bootstrap command argument handling without touching PostgreSQL.
- [ ] Integration tests for bootstrap against disposable PostgreSQL.

**Dependencies:** Phase 1.

**Estimated scope:** Medium.

### Task 2.2: Session Login and Logout

**Description:** Add login, logout, secure cookies, and session storage.

**Acceptance criteria:**

- [ ] Valid admin can log in.
- [ ] Invalid credentials fail without leaking user existence.
- [ ] Cookies are httpOnly, secure in production, and sameSite.
- [ ] Logout invalidates the session.

**Verification:**

- [ ] Auth handler tests pass.
- [ ] Browser smoke test covers login/logout.

**Dependencies:** Task 2.1.

**Estimated scope:** Medium.

### Task 2.3: RBAC Permission Checks

**Description:** Add roles, permissions, and resource authorization helpers.

**Status:** Default roles, permissions, authorization helper, and idempotent RBAC seed migration completed. Handler middleware and resource ownership checks are pending.

**Acceptance criteria:**

- [x] Owner/admin roles are seeded.
- [x] Permission checks can guard services.
- [x] Reseller/customer roles are reserved in the initial permission model.
- [ ] Permission checks can guard HTTP handlers.
- [ ] Resource ownership checks are implemented for account-owned resources.
- [ ] Unauthorized actions return structured errors.

**Verification:**

- [x] Unit tests cover allow, deny, and missing role.
- [ ] Integration test confirms RBAC seed migration on disposable PostgreSQL.

**Dependencies:** Task 2.2.

**Estimated scope:** Medium.

### Task 2.4: Audit Event Pipeline

**Description:** Add a standard audit writer and audit listing API/UI.

**Status:** Audit writer and SQL store foundation completed. Wiring security-sensitive actions and listing API/UI are pending.

**Acceptance criteria:**

- [x] Audit writer can persist security-sensitive event records.
- [x] Audit event includes actor, action, target, IP, user agent, metadata, and timestamp.
- [ ] Security-sensitive actions create audit events.
- [ ] Admin can view recent audit events.

**Verification:**

- [x] Unit tests verify audit validation and write behavior without touching PostgreSQL.
- [ ] Integration tests verify audit events are written.

**Dependencies:** Task 2.3.

**Estimated scope:** Medium.

## Phase 3: Agent and Job System

### Task 3.1: Agent Service Skeleton

**Description:** Implement `motekar-agent` with health, capabilities, and local communication endpoint.

**Acceptance criteria:**

- [ ] Agent starts as a long-running process.
- [ ] Agent exposes health and capabilities.
- [ ] Panel can query local agent status.

**Verification:**

- [ ] Agent unit tests pass.
- [ ] Panel-agent integration smoke test passes.

**Dependencies:** Phase 1.

**Estimated scope:** Medium.

### Task 3.2: Agent Action Registry

**Description:** Add typed allowlisted actions with payload validation.

**Status:** Initial safe registry completed with `agent.health` and `agent.capabilities`. Privileged OS actions are intentionally not implemented yet.

**Acceptance criteria:**

- [x] Unknown action is rejected.
- [x] Invalid JSON payload is rejected at HTTP boundary.
- [x] Actions return structured result and logs.
- [x] No action accepts raw shell strings.
- [ ] Future privileged actions include explicit payload validation per action.

**Verification:**

- [x] Unit tests cover valid action and unknown action.
- [x] HTTP handler tests cover action execution and unknown action.
- [x] `make test` passes.
- [x] `make build` passes.

**Dependencies:** Task 3.1.

**Estimated scope:** Medium.

### Task 3.3: PostgreSQL Job Queue

**Description:** Implement job enqueue, claim, run, retry, fail, succeed, and lock behavior.

**Status:** Queue contract, retry policy, and PostgreSQL store foundation completed. Live PostgreSQL concurrency/locking verification and worker runtime are pending.

**Acceptance criteria:**

- [x] Panel can enqueue a job through queue service.
- [x] Worker can claim one job through SQL store using `FOR UPDATE SKIP LOCKED`.
- [x] Job logs are stored for failed jobs.
- [x] Failed jobs are visible through persisted status.
- [x] Conflicting jobs can be locked by resource key at SQL claim time.
- [ ] Worker runtime executes claimed jobs.

**Verification:**

- [x] Unit tests cover enqueue validation and retry/final failure policy.
- [ ] Integration tests with PostgreSQL pass.

**Dependencies:** Task 3.2.

**Estimated scope:** Large; split if needed.

### Task 3.4: Job UI

**Description:** Add admin UI for job list, status detail, logs, retry, and cancel where safe.

**Acceptance criteria:**

- [ ] Admin can see queued/running/failed/succeeded jobs.
- [ ] Admin can inspect job logs.
- [ ] Admin can retry failed retryable jobs.

**Verification:**

- [ ] Handler tests.
- [ ] Browser smoke test.

**Dependencies:** Task 3.3.

**Estimated scope:** Medium.

## Phase 4: Ubuntu 24.04 Installer and Server Inventory

### Task 4.1: OS Detection

**Description:** Implement OS detection that accepts Ubuntu 24.04 LTS and rejects unsupported systems.

**Status:** Safe parser/checker completed with fixture unit tests. Installer integration pending.

**Acceptance criteria:**

- [x] Ubuntu 24.04 LTS is detected as supported.
- [x] Non-Ubuntu systems are rejected in first installer release.
- [x] Error message mentions planned Debian/RHEL support.
- [ ] Installer command uses this detector during preflight.

**Verification:**

- [x] Unit tests with fixture `/etc/os-release` content.
- [x] `make test` passes.
- [x] `make build` passes.

**Dependencies:** Phase 1.

**Estimated scope:** Small.

### Task 4.2: Installer Preflight Checks

**Description:** Add checks for CPU, RAM, disk, swap, root privileges, systemd, ports, and PostgreSQL availability/install plan.

**Status:** Host-safe profile-aware preflight logic completed with fixture-style facts. Real host collector and VPS manual verification are pending.

**Acceptance criteria:**

- [x] Preflight reports pass/fail for each requirement.
- [x] Minimum server requirements match spec.
- [x] Single-user profile allows 1 GB RAM with warning and stronger swap requirement.
- [x] Shared-hosting profile still blocks 1 GB RAM.
- [x] Unsafe install state blocks installation.
- [x] Host-safe sample CLI exists.
- [ ] Real installer collector gathers facts on disposable Ubuntu 24.04 VPS.

**Verification:**

- [x] Unit tests for check logic.
- [x] `make test` passes.
- [x] `make build` passes.
- [x] `motekarctl preflight sample` reports all sample checks as pass.
- [ ] Manual test on Ubuntu 24.04 environment.

**Dependencies:** Task 4.1.

**Estimated scope:** Medium.

### Task 4.3: Immutable Web Server Selection

**Description:** Add installer flow to select Nginx or Apache once and persist it as immutable server setting.

**Status:** Domain policy and SQL store guard completed. Installer command, audit event, and live PostgreSQL verification are pending.

**Acceptance criteria:**

- [x] Supported web server values are validated.
- [x] Selected web server is saved through immutable setting policy.
- [x] Store-level guard prevents overwriting an already selected immutable value.
- [ ] Installer requires web server selection.
- [ ] Panel cannot change it after install through public API/UI.
- [ ] Attempt to change setting is rejected and audited.

**Verification:**

- [x] Unit tests for web server validation and immutable selection policy.
- [ ] Integration tests for SQL setting immutability on disposable PostgreSQL.

**Dependencies:** Task 4.2.

**Estimated scope:** Medium.

### Task 4.4: Server Inventory Dashboard

**Description:** Add dashboard for OS, CPU, RAM, disk, load, IPs, selected web server, services, and agent status.

**Acceptance criteria:**

- [ ] Admin can view server overview.
- [ ] Data comes from agent/inventory APIs.
- [ ] Unsupported/missing data is shown clearly.

**Verification:**

- [ ] Handler tests.
- [ ] Browser smoke test.

**Dependencies:** Tasks 3.1, 4.3.

**Estimated scope:** Medium.

## Phase 5: Hosting Accounts and Packages

### Task 5.1: Account and Package Schema

**Description:** Add hosting accounts, packages, quota fields, and ownership model.

**Acceptance criteria:**

- [ ] Admin can define package limits.
- [ ] Admin can create hosting account assigned to package.
- [ ] Account has owner/user relationship.

**Verification:**

- [ ] Integration tests for package/account CRUD.

**Dependencies:** Phase 2.

**Estimated scope:** Medium.

### Task 5.2: Linux User and Account Root Provisioning

**Description:** Add agent action to create account Linux user, group, and home/site root.

**Acceptance criteria:**

- [ ] Account provisioning creates isolated filesystem root.
- [ ] Permissions prevent cross-account access.
- [ ] Action is idempotent.
- [ ] Audit and job logs are recorded.

**Verification:**

- [ ] System test on Ubuntu 24.04.

**Dependencies:** Tasks 3.3, 5.1.

**Estimated scope:** Large; split if needed.

### Task 5.3: Disk Quota Enforcement

**Description:** Add native quota/project quota support where available.

**Acceptance criteria:**

- [ ] Package disk limit can be applied.
- [ ] Usage can be reported.
- [ ] Failure to apply quota blocks account activation.

**Verification:**

- [ ] System test on Ubuntu 24.04 filesystem target.

**Dependencies:** Task 5.2.

**Estimated scope:** Large.

### Task 5.4: cgroups/systemd Resource Limits

**Description:** Add package CPU/RAM/process limit application through systemd/cgroups.

**Acceptance criteria:**

- [ ] Runtime services can inherit account limits.
- [ ] Limits are visible in account detail.
- [ ] Package update reconciles limits.

**Verification:**

- [ ] System test with sample service.

**Dependencies:** Task 5.2.

**Estimated scope:** Large.

## Phase 6: Website Hosting

### Task 6.1: Website and Domain Schema

**Description:** Add websites, domains, aliases, document roots, and runtime profile data model.

**Acceptance criteria:**

- [ ] Account can own websites and domains.
- [ ] Domain uniqueness is enforced.
- [ ] Runtime profile can be assigned.

**Verification:**

- [ ] Integration tests for model constraints.

**Dependencies:** Phase 5.

**Estimated scope:** Medium.

### Task 6.2: Nginx Adapter

**Description:** Implement Nginx config rendering, validation, enable/disable, and reload actions.

**Acceptance criteria:**

- [ ] Nginx vhost config is deterministic.
- [ ] Config validation runs before reload.
- [ ] Failed validation does not reload Nginx.
- [ ] Previous config snapshot is preserved.

**Verification:**

- [ ] Unit tests for config rendering.
- [ ] System test with Nginx on Ubuntu 24.04.

**Dependencies:** Tasks 3.3, 6.1.

**Estimated scope:** Large.

### Task 6.3: Apache Adapter

**Description:** Implement Apache vhost rendering, validation, enable/disable, and reload actions.

**Acceptance criteria:**

- [ ] Apache vhost config is deterministic.
- [ ] Config validation runs before reload.
- [ ] Failed validation does not reload Apache.
- [ ] Previous config snapshot is preserved.

**Verification:**

- [ ] Unit tests for config rendering.
- [ ] System test with Apache on Ubuntu 24.04.

**Dependencies:** Tasks 3.3, 6.1.

**Estimated scope:** Large.

### Task 6.4: Static Site Flow

**Description:** Add first end-to-end website creation flow for static sites.

**Acceptance criteria:**

- [ ] Admin/user can create static website.
- [ ] Document root is created.
- [ ] Web server config is applied through selected adapter.
- [ ] Site status is visible.

**Verification:**

- [ ] E2E browser test.
- [ ] System test performs local HTTP request.

**Dependencies:** Task 6.2 or 6.3.

**Estimated scope:** Medium.

## Phase 7: PHP Hosting

### Task 7.1: PHP Version Inventory

**Description:** Detect installed PHP versions and expose available versions to panel.

**Acceptance criteria:**

- [ ] Agent reports installed PHP versions.
- [ ] Panel shows available PHP versions.
- [ ] Unsupported versions are not selectable.

**Verification:**

- [ ] Unit tests for version parser.
- [ ] System test on Ubuntu 24.04.

**Dependencies:** Phase 6.

**Estimated scope:** Medium.

### Task 7.2: PHP-FPM Pool Provisioning

**Description:** Create PHP-FPM pool per site/account with package limits.

**Acceptance criteria:**

- [ ] Pool runs under account user.
- [ ] PHP version is selectable per site.
- [ ] memory/upload/execution limits are applied.
- [ ] Web server routes PHP requests to correct pool.

**Verification:**

- [ ] System test runs `phpinfo` or simple PHP script per site.

**Dependencies:** Task 7.1.

**Estimated scope:** Large.

### Task 7.3: Composer Integration

**Description:** Add controlled Composer actions inside account scope.

**Acceptance criteria:**

- [ ] Composer install/update can run as job.
- [ ] Job runs as account user.
- [ ] Logs are visible.

**Verification:**

- [ ] System test with sample PHP project.

**Dependencies:** Task 7.2.

**Estimated scope:** Medium.

## Phase 8: Database Management

### Task 8.1: MariaDB/MySQL Adapter

**Description:** Add database server detection and management for MariaDB/MySQL.

**Acceptance criteria:**

- [ ] Detect installed database server.
- [ ] Create database.
- [ ] Create database user.
- [ ] Grant least privilege.
- [ ] Rotate password.

**Verification:**

- [ ] Integration/system tests with MariaDB on Ubuntu 24.04.

**Dependencies:** Phase 5.

**Estimated scope:** Large.

### Task 8.2: Database Import/Export

**Description:** Add dump, import, backup, and restore jobs.

**Acceptance criteria:**

- [ ] Export creates downloadable dump.
- [ ] Import runs as job with logs.
- [ ] Restore failure is visible and does not hide partial state.

**Verification:**

- [ ] System test import/export round trip.

**Dependencies:** Task 8.1.

**Estimated scope:** Medium.

## Phase 9: SSL and DNS

### Task 9.1: ACME HTTP-01 SSL

**Description:** Add Let's Encrypt issuance and renewal through HTTP-01.

**Acceptance criteria:**

- [ ] Certificate can be issued for a website.
- [ ] Renewal job is scheduled.
- [ ] Expiry is visible.
- [ ] Force HTTPS can be enabled.

**Verification:**

- [ ] Staging ACME test where possible.
- [ ] Unit tests for certificate state handling.

**Dependencies:** Phase 6.

**Estimated scope:** Large.

### Task 9.2: DNS Zone Editor

**Description:** Add DNS zone and record management with PowerDNS or BIND adapter selected by implementation decision.

**Acceptance criteria:**

- [ ] Admin can create zone.
- [ ] User can manage allowed records for owned domains.
- [ ] SPF/DKIM/DMARC helper records are supported.

**Verification:**

- [ ] Integration tests for zone/record CRUD.
- [ ] Adapter system test.

**Dependencies:** Phase 5.

**Estimated scope:** Large.

## Phase 10: File Manager, Cron, Backup

### Task 10.1: Scoped File Manager

**Description:** Add file operations constrained to account roots.

**Acceptance criteria:**

- [ ] Upload/download/list works inside account root.
- [ ] Path traversal is rejected.
- [ ] Delete/move/copy/archive actions are audited.

**Verification:**

- [ ] Security tests for path traversal.
- [ ] Browser smoke test.

**Dependencies:** Phase 5.

**Estimated scope:** Large.

### Task 10.2: Cron Job Management

**Description:** Add account-scoped scheduled tasks.

**Acceptance criteria:**

- [ ] User can create cron job within package limit.
- [ ] Job runs as account user.
- [ ] Logs/status are visible where possible.

**Verification:**

- [ ] System test with sample scheduled command.

**Dependencies:** Phase 5.

**Estimated scope:** Medium.

### Task 10.3: Backup and Restore Foundation

**Description:** Add account backup jobs for website files and databases.

**Acceptance criteria:**

- [ ] Full account backup job can run.
- [ ] Backup snapshot metadata is stored.
- [ ] Restore preview exists before destructive restore.

**Verification:**

- [ ] System test backup/restore round trip.

**Dependencies:** Phases 6 and 8.

**Estimated scope:** Large.

## Phase 11: Mail Hosting

### Task 11.1: Mail Stack Adapter

**Description:** Add Postfix/Dovecot/Rspamd or SpamAssassin adapter design and detection.

**Acceptance criteria:**

- [ ] Mail stack capability detection exists.
- [ ] Mail domain model exists.
- [ ] Mailbox model exists.

**Verification:**

- [ ] Unit tests for mail model and capability detection.

**Dependencies:** Phase 5.

**Estimated scope:** Large.

### Task 11.2: Mail Domain and Mailbox Provisioning

**Description:** Create mail domain, mailbox, alias, forwarder, and quota workflows.

**Acceptance criteria:**

- [ ] Mailbox can be created.
- [ ] Mailbox quota is applied.
- [ ] Alias and forwarder can be created.
- [ ] DKIM/SPF/DMARC helper guidance exists.

**Verification:**

- [ ] System test for local delivery where feasible.

**Dependencies:** Task 11.1.

**Estimated scope:** Large.

## Phase 12: Security Center and Observability

### Task 12.1: Firewall Adapter

**Description:** Add Ubuntu 24.04 firewall adapter and UI for allowed services.

**Acceptance criteria:**

- [ ] Admin can view firewall state.
- [ ] Admin can allow/deny supported service ports.
- [ ] Dangerous rule changes require confirmation and audit.

**Verification:**

- [ ] System test on Ubuntu 24.04.

**Dependencies:** Phase 4.

**Estimated scope:** Medium.

### Task 12.2: Fail2ban/CrowdSec Integration

**Description:** Add security event integration and basic protection status.

**Acceptance criteria:**

- [ ] Detection of installed protection service.
- [ ] Status visible in Security Center.
- [ ] Basic ban/unban listing where supported.

**Verification:**

- [ ] Adapter tests.

**Dependencies:** Task 12.1.

**Estimated scope:** Medium.

### Task 12.3: Notifications

**Description:** Add notification channels for failed services, failed jobs, high resource usage, and certificate expiry.

**Acceptance criteria:**

- [ ] Email notification channel exists.
- [ ] Webhook notification channel exists.
- [ ] Alert events are auditable.

**Verification:**

- [ ] Unit tests for notification dispatch.

**Dependencies:** Phase 3.

**Estimated scope:** Medium.

## Phase 13: WordPress, Git Deploy, Branding, API

### Task 13.1: WordPress Toolkit

**Description:** Add WordPress install, clone/staging, backup, and security checks.

**Acceptance criteria:**

- [ ] User can install WordPress into a site.
- [ ] User can clone/stage a WordPress site.
- [ ] Security checks are visible.

**Verification:**

- [ ] System test with WordPress install.

**Dependencies:** Phases 7, 8, 9, 10.

**Estimated scope:** Large.

### Task 13.2: Git-Based Deployment

**Description:** Add deployment from Git repository into account site.

**Acceptance criteria:**

- [ ] User can connect repository URL.
- [ ] Deploy job checks out code as account user.
- [ ] Build command can run with logs.

**Verification:**

- [ ] System test with sample repository.

**Dependencies:** Phase 6.

**Estimated scope:** Large.

### Task 13.3: Branding and Multi-Language Readiness

**Description:** Add logo/color/company name settings, dark mode, mobile layout, and translation-ready UI strings.

**Acceptance criteria:**

- [ ] Admin can set panel branding.
- [ ] UI supports dark mode.
- [ ] UI uses translation key structure.
- [ ] Core views are mobile responsive.

**Verification:**

- [ ] Browser screenshots desktop/mobile.

**Dependencies:** Phase 2.

**Estimated scope:** Medium.

### Task 13.4: Public API and Provisioning Hooks

**Description:** Add API tokens, OpenAPI docs, webhooks, and WHMCS-style account provisioning readiness.

**Acceptance criteria:**

- [ ] Scoped API tokens can be created.
- [ ] Account provisioning endpoint exists.
- [ ] Webhook delivery exists for account/job events.
- [ ] OpenAPI spec is generated or maintained.

**Verification:**

- [ ] API integration tests.

**Dependencies:** Phases 2 and 5.

**Estimated scope:** Large.

## Checkpoints

### Checkpoint A: Foundation Ready

After Phases 0-4:

- [ ] Binaries build.
- [ ] PostgreSQL migrations pass.
- [ ] Panel starts.
- [ ] Agent starts.
- [ ] Login works.
- [ ] Jobs work.
- [ ] Ubuntu 24.04 preflight works.
- [ ] Web server selection is immutable.

### Checkpoint B: Shared Hosting Core Ready

After Phases 5-8:

- [ ] Accounts and packages work.
- [ ] Account isolation works.
- [ ] Static website hosting works.
- [ ] PHP hosting works.
- [ ] Database management works.
- [ ] Core workflows are audited.

### Checkpoint C: Production Hosting Ready

After Phases 9-12:

- [ ] SSL works.
- [ ] DNS works.
- [ ] Backups work.
- [ ] Mail works.
- [ ] Security center works.
- [ ] Notifications work.

### Checkpoint D: Provider Product Ready

After Phase 13:

- [ ] WordPress toolkit works.
- [ ] Git deployment works.
- [ ] Branding works.
- [ ] Public API/provisioning hooks work.
- [ ] Documentation and runbooks exist.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Native OS isolation is weaker than containers | High | Use Linux users, filesystem permissions, cgroups, quotas, PHP-FPM pools, and strict agent allowlists |
| Mail hosting is operationally complex | High | Treat mail as first-class system with dedicated tests, DNS helpers, logs, anti-spam, and abuse controls |
| Web server adapter drift | Medium | Immutable install-time selection and deterministic config rendering |
| Ubuntu-only first release limits adoption | Medium | Keep OS adapter boundaries from day one and add Debian/RHEL after core is stable |
| Agent privilege mistakes | High | No raw shell strings, action registry, schema validation, audit logs, and system tests |
| Scope is very large | High | Build in dependency order with checkpoints, not feature-by-feature chaos |

## Immediate Next Tasks

Completed:

1. Task 0.1: Initialize Repository Structure.
2. Task 0.2: Add Development Commands.
3. Task 1.2: Configuration Loader.
4. Task 1.3: Structured Logging.
5. Task 1.4: HTTP Server Skeleton.

Next:

1. Task 1.1: PostgreSQL Configuration and Migrations.
2. Task 2.1: First Admin Bootstrap.
3. Task 3.1: Agent Service Skeleton hardening for local-only communication.
