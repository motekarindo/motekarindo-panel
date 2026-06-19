# Spec: Motekar Panel Server Control Panel

## Status

Draft for review.

## Objective

Build a full server control panel for Linux hosting operations, comparable in product category to cPanel, Plesk, Webuzo, aaPanel, and similar platforms, while using a modular architecture that can grow into multi-server, reseller, SaaS, and plugin-based deployments.

The product must manage web hosting workloads safely through a browser UI and API, without exposing raw shell access to end users. The system must support privileged server operations through a constrained agent, strong audit logging, and service-specific adapters.

Success means an operator can install the panel on supported Linux servers, manage users, websites, DNS, databases, email, SSL, backups, security controls, server services, logs, and updates from one interface, while developers can add new service modules without rewriting the core.

Project name: `Motekar Panel`.

Owner: Motekar Teknologi Indonesia.

## Target Users

- Server owners managing one or more VPS or dedicated servers.
- Hosting providers managing customers, plans, packages, and reseller accounts.
- Developers deploying PHP, Node.js, static, and database-backed sites.
- Agencies managing many client websites.
- Sysadmins who need repeatable operations, auditability, and recovery.

## Product Principles

- Lightweight by default: the panel must run acceptably on small VPS instances and avoid unnecessary resident services.
- Security-first: every privileged operation must be explicit, auditable, and constrained.
- Modular by design: web server, DNS, database, mail, backup, and firewall implementations must be adapters behind stable interfaces.
- Full scope, incremental delivery: the spec covers the complete product direction; implementation can be sequenced without changing the target architecture.
- No raw shell from UI by default: all operations go through typed jobs and allowlisted agent actions.
- Reversible operations where practical: config changes should be versioned and rollback-capable.
- Operational clarity: every background job has status, logs, retry policy, and operator-visible result.
- Distribution-friendly: installable on fresh servers and maintainable through upgrades.

## Supported Platforms

## Minimum Server Requirements

The product must be designed for low-resource VPS deployments.

Minimum supported single-server profile:

- 1 vCPU.
- 2 GB RAM.
- 20 GB disk.
- 1 GB swap.
- Ubuntu 24.04 LTS.

Recommended single-server profile for production with web, database, DNS, and mail enabled:

- 2 vCPU.
- 4 GB RAM.
- 40 GB disk or more.
- 2 GB swap.
- Ubuntu 24.04 LTS.

Design constraints:

- PostgreSQL is the only required external data service for the default install.
- Redis must be optional, not required for the default install.
- The queue should use PostgreSQL-backed jobs by default.
- The panel web process and local agent should be lightweight resident services.
- Optional services should be enabled by feature need, not installed unconditionally.

### Initial Linux Targets

- Ubuntu 24.04 LTS.

### Future Linux Targets

- Debian 12.
- Future Ubuntu LTS releases after validation.
- AlmaLinux.
- Rocky Linux.
- RHEL-compatible distributions.

### RHEL-Compatible Support Plan

Motekar Panel should support RHEL-compatible distributions such as Rocky Linux and AlmaLinux, but they should be treated as a separate support track from Debian/Ubuntu.

Required RHEL-compatible work:

- Package manager adapter for `dnf`/`yum`.
- Service adapter differences for package names, service names, filesystem paths, and default config locations.
- SELinux-aware install and runtime policies.
- firewalld adapter in addition to UFW/nftables.
- EPEL/third-party repository handling policy for packages not available in base repositories.
- PHP multi-version strategy compatible with RHEL-family packaging.
- Mail stack packaging validation for Postfix, Dovecot, Rspamd/SpamAssassin, and DKIM tooling.
- Automated install tests on Rocky Linux and AlmaLinux before marking support as stable.

Support policy:

- Ubuntu 24.04 LTS is the first supported installation target.
- Debian 12 is planned after Ubuntu 24.04 LTS support is stable.
- Rocky Linux and AlmaLinux are planned supported targets after the OS adapter layer is stable.
- RHEL-compatible support must not be declared production-ready until installer, service adapters, firewall, SELinux behavior, web server config, PHP, database, DNS, mail, SSL, backup, and upgrade flows pass system tests.

### System Assumptions

- systemd is available.
- A package manager is available through apt or dnf/yum adapters.
- The panel runs on a server it manages, with later support for remote managed nodes.
- IPv4 is required; IPv6 support is first-class but can be disabled by deployment policy.

## High-Level Architecture

```text
Browser UI
  |
  v
Panel Web Application
  - Admin UI
  - Public API
  - Auth/RBAC
  - Audit log
  - Job orchestration
  |
  v
PostgreSQL-backed Queue / Scheduler
  |
  v
Privileged Agent
  - Local system operations
  - Service adapters
  - Config rendering
  - Health checks
  |
  v
Linux Services
  - Nginx / Apache / Caddy
  - PHP-FPM / Node.js / Python
  - MariaDB / MySQL / PostgreSQL
  - PowerDNS / BIND
  - Postfix / Dovecot / Rspamd
  - UFW / nftables / firewalld
  - Fail2ban / CrowdSec
  - Certbot / ACME client
  - Restic / Borg / rclone
```

## Core Components

### 1. Panel Web Application

Responsibilities:

- Render dashboard and management UI.
- Expose REST API for internal UI, CLI, and future integrations.
- Authenticate users and enforce RBAC.
- Validate all user input at API boundaries.
- Persist product state in the panel database.
- Dispatch background jobs instead of performing privileged work inline.
- Store audit events for every user, system, and agent action.

The web application must never directly concatenate shell commands from user input.

Architecture decision:

- The main panel web application should be written in Go.
- The UI should be a custom operational dashboard, server-rendered where practical, with progressive enhancement for interactive workflows.
- The UI must be tailored to the panel's hosting workflows rather than built as a generic CRUD admin framework.
- Laravel and large admin frameworks remain valid alternatives for other products, but are not the default for Motekar Panel because the target includes low-spec VPS installations.

### 2. Privileged Agent

Responsibilities:

- Run as a system service with least privilege possible for supported operations.
- Execute allowlisted actions only.
- Validate job payload schemas before execution.
- Render and validate service configuration before applying it.
- Perform atomic config writes where possible.
- Report structured job logs and result codes.
- Expose local health and capability information to the panel.

The agent should be implemented as a separate Go process with a strict API boundary. Go is the default because distribution, static binaries, system APIs, and operational tooling are straightforward.

### Installation-Time Web Server Selection

The primary web server is selected once during installation and cannot be changed from the panel after installation.

Allowed installation choices:

- Nginx.
- Apache.

Future installer choices may include Caddy or OpenLiteSpeed only after they have complete adapter support.

Rationale:

- The web server owns critical HTTP/HTTPS behavior for every account.
- Switching after websites, SSL certificates, redirects, rewrites, reverse proxies, and custom configs exist is high risk.
- Shared hosting accounts should inherit the server owner's web server decision.
- End users should choose runtime and application profile, not the underlying web server engine.

Policy:

- No per-domain web server selection.
- No per-account web server selection.
- No in-panel web server migration button.
- Changing web server requires reinstalling the server or using a future documented offline migration runbook.
- The installer must persist the selected web server as a server-level immutable setting.

### 3. Job System

Responsibilities:

- Execute long-running operations asynchronously.
- Track state: queued, running, succeeded, failed, cancelled, retrying.
- Capture stdout/stderr-like structured logs without exposing secrets.
- Support idempotency keys for operations that may be retried.
- Support per-job timeout, retry, and cancellation semantics.
- Prevent conflicting jobs on the same resource through locks.

Default implementation:

- Store jobs in PostgreSQL.
- Use PostgreSQL locking semantics for worker coordination.
- Keep Redis optional for larger deployments that need separate queue infrastructure.

Examples:

- Create website.
- Issue SSL certificate.
- Restore backup.
- Change PHP version.
- Restart service.
- Create mailbox.
- Rotate database password.

### 4. Panel Database

The panel database stores panel state, not necessarily customer workload data.

Recommended default:

- PostgreSQL for panel state.

Supported future modes:

- External PostgreSQL for HA or managed deployments.

SQLite is not a supported default because audit logs, resource ownership, job state, and future multi-node control plane behavior are core requirements.

Core data domains:

- Users and organizations.
- Roles and permissions.
- Servers and nodes.
- Hosting accounts.
- Domains and websites.
- Databases and database users.
- Mail domains, mailboxes, aliases.
- DNS zones and records.
- SSL certificates.
- Backups and restore points.
- Jobs and job logs.
- Audit events.
- Service inventory and health checks.
- Licenses and feature flags if commercialized.

## Module System

The product must use module boundaries so implementations can be swapped.

### Module Categories

- Web server module: Nginx, Apache, Caddy.
- Runtime module: PHP-FPM, Node.js, Python, static.
- Database module: MariaDB, MySQL, PostgreSQL.
- DNS module: PowerDNS, BIND, external DNS providers.
- Mail module: Postfix, Dovecot, Rspamd, external SMTP.
- SSL module: ACME providers, custom certificates.
- Backup module: local disk, S3-compatible storage, SSH/SFTP, rclone remotes.
- Firewall module: UFW, nftables, firewalld.
- Security module: Fail2ban, CrowdSec, malware scanner.
- File module: local filesystem, jailed file manager, SFTP.
- Installer module: OS package manager adapters.
- Monitoring module: local metrics, remote telemetry, alert channels.

### Adapter Contract

Every service adapter should expose:

- `detect()`: determine whether the service exists and what version is installed.
- `install()`: install service packages.
- `configure()`: render managed config.
- `validate()`: verify config before activation.
- `reload()`: reload without full restart where supported.
- `restart()`: restart the service.
- `status()`: return health and version information.
- `backupConfig()`: snapshot managed configuration.
- `restoreConfig()`: rollback managed configuration.

### Plugin Contract

Plugins may add:

- UI routes.
- API routes under a namespaced prefix.
- Agent actions.
- Job types.
- Database migrations.
- Dashboard widgets.
- Notification channels.
- Service adapters.

Plugins must declare:

- Permissions required.
- Agent capabilities required.
- Database migrations.
- Config files managed.
- External network endpoints used.
- Upgrade and rollback hooks.

Plugins must not run arbitrary code as root without explicit registration and review.

## Feature Domains

### A. Authentication and Identity

Capabilities:

- Admin login.
- Session-based authentication with secure cookies.
- Optional TOTP 2FA.
- Recovery codes.
- Password reset.
- API tokens with scoped permissions.
- Login history.
- Device/session management.
- Optional SSO through OIDC or SAML in advanced editions.

Security requirements:

- Passwords hashed with Argon2id or bcrypt with strong parameters.
- Sessions stored server-side or signed/encrypted with rotation support.
- Cookies must be httpOnly, secure, and sameSite.
- Brute force protection for login.
- Audit events for login, logout, failed login, password reset, 2FA changes.

### B. Authorization and Tenancy

Core concepts:

- System owner.
- Admin.
- Reseller.
- Customer.
- Site/application operator.
- Read-only auditor.

Resources:

- Server.
- Hosting account.
- Website.
- Domain.
- Database.
- Mail domain.
- Mailbox.
- DNS zone.
- Backup.
- Job.

Authorization model:

- RBAC for standard roles.
- Optional scoped permissions for API tokens and custom roles.
- Resource ownership checks on every API operation.
- No resource access by ID alone without authorization.

### C. Server Overview and Inventory

Capabilities:

- CPU, RAM, disk, load average.
- Network interfaces and IP addresses.
- OS version and kernel.
- Package updates.
- Service status.
- Disk usage by account and site.
- Process overview.
- Reboot required detection.
- Time sync status.
- Filesystem mounts.

Metrics:

- Current value.
- Short history.
- Alert thresholds.
- Per-node status in future multi-node mode.

### D. Website and Application Hosting

Capabilities:

- Create website.
- Add domains and aliases.
- Set document root.
- Enable/disable website.
- Redirects.
- Force HTTPS.
- Custom error pages.
- Static site hosting.
- PHP application hosting.
- Node.js application hosting.
- Reverse proxy application hosting.
- Per-site environment variables.
- Per-site logs.
- Per-site resource limits where supported.
- Staging site and clone support.

Web server support:

- Nginx first.
- Apache adapter.
- Caddy adapter.

Config requirements:

- Managed config sections must be clearly marked.
- Config validation must run before reload.
- Reload failure must preserve previous working config.
- Custom user config must be isolated from generated base config.

### E. PHP Management

Capabilities:

- Install supported PHP versions.
- Select PHP version per site.
- Manage PHP-FPM pools.
- Configure memory limit, upload limit, post limit, execution time.
- Enable/disable extensions.
- Composer integration.
- Per-site `php.ini` overrides where safe.
- Display runtime health.

Security requirements:

- Separate Linux user or pool identity per hosting account where feasible.
- Restrict cross-account file access.
- Disable dangerous functions by policy where configured.

### F. Node.js and App Runtime Management

Capabilities:

- Create Node.js app.
- Select Node version.
- Install dependencies.
- Define start command.
- Manage process through systemd or process manager adapter.
- Reverse proxy from web server.
- Environment variable management.
- Build command support.
- Logs and restart controls.

Future runtime adapters:

- Python WSGI/ASGI.
- Ruby.
- Static build deployment.

Container-based application hosting is out of scope for the core product. Motekar Panel should manage native OS services and runtimes by default.

### G. Database Management

Capabilities:

- Install and manage MariaDB/MySQL.
- Optional PostgreSQL workload database support.
- Create database.
- Create database user.
- Assign privileges.
- Rotate passwords.
- Import/export database.
- Backup database.
- Restore database.
- View database size.
- Optional phpMyAdmin/Adminer integration behind panel auth.

Security requirements:

- Database passwords generated with secure randomness.
- No database password exposure after initial display unless rotated.
- Secrets encrypted at rest when stored.
- Least privilege grants by default.

### H. DNS Management

Capabilities:

- DNS zone creation.
- A, AAAA, CNAME, MX, TXT, SRV, CAA, NS records.
- DKIM, SPF, DMARC helper records.
- Zone import/export.
- Zone templates.
- DNSSEC support where backend allows.
- External DNS provider adapters.

DNS backends:

- PowerDNS.
- BIND.
- Cloudflare provider.
- Route53 provider.
- Generic webhook/API provider in future.

### I. Mail Hosting

Mail hosting is included in the full product scope and should be part of the first public release plan if the supported OS and dependency stack can satisfy the reliability and security requirements.

Capabilities:

- Mail domain management.
- Mailbox creation.
- Alias management.
- Forwarders.
- Catch-all policy.
- IMAP/SMTP configuration display.
- Webmail integration.
- Quotas.
- Spam filtering.
- DKIM signing.
- SPF/DMARC assistant.
- Mail queue viewer.
- Mail logs.

Mail stack:

- Postfix for SMTP.
- Dovecot for IMAP/POP3.
- Rspamd or SpamAssassin adapter.
- OpenDKIM or Rspamd DKIM support.

Security requirements:

- TLS enforced where possible.
- No open relay.
- Rate limits for outbound mail.
- Password hashing for mail users.
- Abuse detection hooks.

### J. SSL and Certificate Management

Capabilities:

- ACME HTTP-01 issuance.
- ACME DNS-01 issuance through DNS adapters.
- Wildcard certificates where DNS adapter supports it.
- Custom certificate upload.
- Certificate renewal scheduler.
- Certificate status and expiry alerts.
- Force HTTPS.
- HSTS policy toggle with warnings.

Safety requirements:

- Validate DNS and web reachability before issuance when possible.
- Never overwrite custom certificates without explicit user action.
- Store private keys with strict filesystem permissions.

### K. File Management

Capabilities:

- Browser-based file manager.
- Upload, download, rename, move, copy, delete.
- Archive and extract.
- File permissions.
- Ownership repair.
- Text editor for safe file types.
- Search within account scope.
- SFTP account management.

Security requirements:

- Path traversal protection.
- Resource scope enforced server-side.
- File operations through agent actions.
- No editing outside authorized account roots.
- Dangerous file operations require confirmation and audit.

### L. Backup and Restore

Capabilities:

- Full account backup.
- Website files backup.
- Database backup.
- Mail backup.
- DNS zone backup.
- Panel configuration backup.
- Scheduled backups.
- Retention policies.
- Local storage.
- S3-compatible storage.
- SSH/SFTP storage.
- rclone remote support.
- Restore to same account.
- Restore to new account.
- Download backup.

Backup engines:

- Restic adapter.
- Borg adapter.
- Tar/mysql dump fallback.

Safety requirements:

- Restore preview before destructive restore.
- Restore jobs must produce detailed logs.
- Backups encrypted when stored off-server.
- Backup credentials stored as secrets.

### M. Security Center

Capabilities:

- Firewall rules.
- SSH hardening guidance.
- Fail2ban/CrowdSec integration.
- Malware scanner integration.
- File integrity checks.
- Login protection.
- IP allowlist/denylist.
- Security update status.
- Exposed service inventory.
- TLS policy checks.
- Permission audit.

Policy examples:

- Disable password SSH login.
- Restrict panel access by IP.
- Enforce 2FA for admins.
- Block repeated login failures.
- Alert on new listening ports.

### N. Service Management

Capabilities:

- List managed services.
- Start, stop, restart, reload.
- Enable/disable at boot.
- View service logs.
- View config validation results.
- Show package version.
- Detect broken services.

Safety requirements:

- High-impact service actions require elevated permission.
- Reload preferred over restart when supported.
- Agent must reject unknown service names.

### O. Logs and Observability

Capabilities:

- Panel audit logs.
- Job logs.
- Web server access/error logs.
- PHP-FPM logs.
- Database logs where accessible.
- Mail logs.
- System logs.
- Security event logs.
- Search and filtering.
- Export logs.
- Alerts and notifications.

Notification channels:

- Email.
- Webhook.
- Slack-compatible webhook.
- Telegram in future.

### P. Package and Update Management

Capabilities:

- Show available system updates.
- Update panel.
- Update agent.
- Update service packages.
- Security update recommendations.
- Maintenance mode.
- Rollback panel release where supported.

Safety requirements:

- Pre-update compatibility checks.
- Backup panel state before major updates.
- Keep update audit events.
- Never auto-upgrade critical services without policy.

### Q. Account, Package, and Reseller Management

Capabilities:

- Hosting accounts.
- Resource packages.
- Disk quota.
- Bandwidth quota where measurable.
- CPU limit.
- RAM limit for app/runtime processes.
- Process limit.
- I/O limit where supported.
- Inode limit where supported.
- Website count limits.
- Database count limits.
- Mailbox count limits.
- Domain and subdomain count limits.
- Cron job count limits.
- Feature flags per package.
- Reseller ownership.
- Account suspension.
- Account transfer between resellers.
- Package upgrade and downgrade.
- Admin impersonation for support workflows.

Package example:

```text
Starter
- Disk: 5 GB
- Runtime RAM: 512 MB
- CPU: 50% of 1 core where supported
- Websites: 5
- Domains/subdomains: 10
- Databases: 5
- Mailboxes: 10
- Cron jobs: 10
- Monthly bandwidth: 100 GB where measurable
```

Resource enforcement:

- Disk quota should use Linux quota, project quota, or filesystem-specific quota support.
- Runtime RAM and CPU limits should use systemd slices/scopes and cgroups.
- PHP limits should be applied through PHP-FPM pool configuration.
- Node.js and Python apps should run as per-app services with cgroup limits.
- Mailbox quotas should be enforced by the mail storage backend.
- Database limits should start with count/ownership/quota policy; hard per-user database RAM limits are not guaranteed on shared database servers.
- Every package change must create an audit event and a reconciliation job to apply limits.

Future commercial features:

- Billing provider integrations.
- Invoice hooks.
- WHMCS-compatible provisioning API.
- License server.

License, billing, and reseller concepts must be designed from the beginning. Implementation can be modular, but the data model and permission model must not block commercial hosting provider workflows.

### T. Competitive Feature Plan

Feature references:

- OpenPanel features: https://openpanel.com/features/
- aaPanel features: https://www.aapanel.com/new/feature.html
- OpenPanel emphasizes per-user resource limits, package upgrade/downgrade, impersonation, activity logs, branding, responsive UI, DNS zone editing, cron jobs, SSL, PHP versions, Node.js/Python app management, database tools, file manager, notifications, and billing integrations.
- aaPanel emphasizes sub-accounts and hosting packages, website management, database management, file manager, app/plugin store, scheduled tasks, mail server, WAF, WordPress toolkit, Git deployment, and mobile-friendly management.

Motekar Panel should adopt the relevant shared hosting lessons while keeping its own architecture constraints:

- Adopt package and quota management per account.
- Adopt suspend/delete account operations.
- Adopt upgrade/downgrade package workflows.
- Adopt admin impersonation with strong audit logging.
- Adopt domain, subdomain, redirect, SSL, PHP manager, reverse proxy, and response log workflows.
- Adopt database create/import/export/backup/restore workflows.
- Adopt file manager upload/download/archive/extract/edit/search workflows.
- Adopt cron job and scheduled task workflows.
- Adopt DNS zone editor.
- Adopt mail server management.
- Adopt WAF/firewall/security center concepts.
- Adopt WordPress toolkit as a future first-party module.
- Adopt Git-based deployment as a future first-party module.
- Adopt activity logs and resource usage reports.
- Adopt branding, dark mode, mobile responsive UI, and multi-language readiness.
- Adopt WHMCS/API-style provisioning readiness.

Motekar Panel should intentionally not adopt:

- Per-user or per-domain web server switching.
- Containerized user services.
- Browser terminal for arbitrary shell access.
- In-panel primary web server migration.

These exclusions are deliberate because Motekar Panel is a native OS shared hosting panel optimized for lower-resource VPS deployments and predictable operations.

### R. API, CLI, and Automation

Public surfaces:

- REST API.
- Admin CLI.
- Agent CLI for diagnostics.
- Webhooks for job events.
- OpenAPI documentation.

API principles:

- Contract-first design.
- Stable resource naming.
- Structured error responses.
- Pagination on list endpoints.
- Idempotency keys for mutating operations.
- Versioned public API path, for example `/api/v1`.

Standard error shape:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid request payload",
    "details": {}
  }
}
```

### S. Multi-Server and Cluster Roadmap

The architecture must allow future multi-node management.

Concepts:

- Control plane node.
- Managed server node.
- Node registration.
- Node capability discovery.
- Per-node agent identity.
- Per-node job queue routing.
- Central audit log.
- Per-node service inventory.

Use cases:

- Manage multiple VPS from one panel.
- Separate DNS/mail/web/database nodes.
- Migrate account between nodes.
- Central backup policy.
- Reseller/provider fleet view.

This must not force the first implementation to be multi-node, but database schema and API boundaries should not block it.

## Data Model Outline

Core entities:

- `User`
- `Role`
- `Permission`
- `Session`
- `ApiToken`
- `Organization`
- `ServerNode`
- `Agent`
- `HostingAccount`
- `Package`
- `Domain`
- `DnsZone`
- `DnsRecord`
- `Website`
- `Runtime`
- `DatabaseServer`
- `Database`
- `DatabaseUser`
- `MailDomain`
- `Mailbox`
- `MailAlias`
- `Certificate`
- `BackupRepository`
- `BackupSnapshot`
- `Job`
- `JobLog`
- `AuditEvent`
- `Service`
- `NotificationChannel`
- `Plugin`
- `Secret`

Important relationships:

- Organization owns users and hosting accounts.
- Hosting account owns websites, domains, databases, mailboxes, and backups.
- Server node has agent, services, IP addresses, and capabilities.
- Website belongs to hosting account and may use multiple domains.
- Job targets a resource and server node.
- Audit event references actor, action, target, and request metadata.

## Security Model

### Trust Boundaries

- Browser to panel API.
- Panel API to database.
- Panel API to queue.
- Queue to agent.
- Agent to operating system.
- Panel to external services.
- Plugin to core.

### Required Controls

- Validate all external input.
- Enforce authorization for every resource operation.
- Use parameterized database queries.
- Use server-side sessions or tightly scoped API tokens.
- Encrypt secrets at rest.
- Never log secrets.
- Use TLS for network communication.
- Use local Unix socket or mutually authenticated channel for local agent communication.
- Use mTLS or signed requests for remote agent communication.
- Store audit logs append-only where practical.
- Apply rate limits to login, API tokens, and sensitive operations.
- Run dependency and secret scanning in CI.

### Agent Hardening

- Agent action registry with explicit schemas.
- No arbitrary shell command execution from API payloads.
- Command arguments passed as arrays, not shell strings.
- Drop privileges for file operations where possible.
- Use account-specific Linux users where feasible.
- Restrict file roots by account.
- Validate service config before applying.
- Snapshot config before modifications.

### Sensitive Data

Sensitive data includes:

- Panel admin passwords.
- API tokens.
- Database passwords.
- Mailbox passwords.
- ACME account keys.
- Certificate private keys.
- Backup repository credentials.
- External provider API keys.

Handling requirements:

- Encrypt when stored.
- Redact in logs and job output.
- Display once where possible.
- Rotate through explicit actions.

## Installation and Lifecycle

### Installer Requirements

- Detect OS and version.
- Check system requirements.
- Install panel web app.
- Install agent service.
- Install database for panel state or connect external DB.
- Configure reverse proxy and TLS for panel.
- Create first admin.
- Register local node.
- Run health checks.

### Uninstall Requirements

- Remove panel services.
- Preserve or optionally remove customer data.
- Export panel state before destructive removal.
- Clearly distinguish panel-managed files from user files.

### Upgrade Requirements

- Database migrations.
- Agent compatibility check.
- Plugin compatibility check.
- Pre-upgrade backup.
- Rollback plan for failed upgrade.

## Commands

The final project must provide commands equivalent to:

```bash
# Development
make dev
make test
make lint
make build

# Panel operations
motekar-panel install
motekar-panel status
motekar-panel upgrade
motekar-panel backup
motekar-panel restore

# Agent diagnostics
motekar-agent status
motekar-agent capabilities
motekar-agent validate-config
```

Exact commands depend on chosen stack and will be finalized in ADRs.

## Project Structure

Recommended monorepo layout:

```text
cmd/
  motekar-panel/          # Main panel web/API binary
  motekar-agent/          # Privileged local agent binary
  motekarctl/             # Operator CLI
internal/
  auth/                   # Authentication, sessions, API tokens
  rbac/                   # Roles, permissions, resource authorization
  audit/                  # Audit event writer and readers
  jobs/                   # PostgreSQL-backed job queue and workers
  server/                 # HTTP server, middleware, routing
  ui/                     # Server-rendered dashboard views and assets
  agent/                  # Agent runtime internals
  adapters/               # Core service adapter interfaces
services/
  migrations/             # PostgreSQL schema migrations
  templates/              # Managed service config templates
plugins/
  web-nginx/
  runtime-php/
  db-mariadb/
  dns-powerdns/
  mail-postfix-dovecot/
docs/
  specs/
  decisions/
  runbooks/
tests/
  integration/
  e2e/
deploy/
  systemd/
  packaging/
  docker-dev/
```

## Code Style

General rules:

- Prefer explicit contracts over implicit behavior.
- Keep module boundaries strict.
- Validate at system boundaries.
- Keep privileged operations out of web request handlers.
- Use structured logs.
- Use typed error codes.
- Keep generated service config deterministic.

Example API service style:

```go
type CreateWebsiteInput struct {
	AccountID  string
	Domain    string
	Runtime   RuntimeKind
}

func (s *WebsiteService) Create(ctx context.Context, actor Actor, input CreateWebsiteInput) (*Website, error) {
	if err := s.permissions.Require(ctx, actor, "website:create", input.AccountID); err != nil {
		return nil, err
	}

	account, err := s.accounts.Get(ctx, input.AccountID)
	if err != nil {
		return nil, err
	}

	website, err := s.websites.Reserve(ctx, ReserveWebsiteInput{
		AccountID: account.ID,
		Domain:    input.Domain,
		Runtime:   input.Runtime,
	})
	if err != nil {
		return nil, err
	}

	if err := s.jobs.Enqueue(ctx, "website.provision", map[string]string{
		"actorId":   actor.ID,
		"websiteId": website.ID,
	}); err != nil {
		return nil, err
	}

	return website, nil
}
```

## Testing Strategy

Testing must protect the developer and operator host machines. Motekar Panel must not run destructive or privileged OS-module tests directly on a host machine.

### Test Levels

- Unit tests for validation, permissions, config rendering, adapter logic.
- Integration tests for API, database, queue, and agent contracts.
- System tests using disposable Linux containers or VMs.
- End-to-end browser tests for critical workflows.
- Security tests for auth, authorization, path traversal, command injection, and secret leakage.
- Upgrade tests for database migrations and agent compatibility.

### Host Safety Rules

Allowed on the host machine:

- Pure unit tests.
- Golden file tests.
- Static validation.
- Build and lint commands.
- Non-privileged API handler tests.

Forbidden on the host machine:

- Installing or removing OS packages.
- Writing to `/etc`, `/var`, `/usr`, `/home`, or system service directories.
- Running `systemctl`, `useradd`, `groupadd`, quota tools, firewall tools, or package managers as part of tests.
- Restarting or reloading real services such as Nginx, Apache, PHP-FPM, MariaDB, PostgreSQL, Postfix, Dovecot, firewalld, UFW, or Fail2ban.
- Modifying firewall, SELinux, users, groups, permissions, quotas, or systemd units.

Required for OS-module tests:

- Disposable Ubuntu 24.04 VM, disposable LXD container, disposable Multipass instance, or dedicated test VPS.
- Explicit environment marker such as `MOTEKAR_TEST_ALLOW_SYSTEM=1`.
- Preflight check that refuses to run on unmarked environments.
- Snapshot, teardown, or rebuild path for every destructive test suite.
- Clear logs of every file, service, and package touched.

### Required Test Scenarios

- Unauthorized user cannot access another account resource.
- Website creation renders valid web server config.
- Invalid config does not reload service.
- Failed job preserves previous working config.
- SSL issuance failure is visible and retryable.
- Backup restore does not escape account root.
- File manager rejects path traversal.
- API token scopes are enforced.
- Plugin cannot register privileged action without declared capability.

## CI/CD Requirements

Quality gates:

- Format check.
- Lint.
- Type check.
- Unit tests.
- Integration tests.
- Build.
- Secret scanning.
- Dependency vulnerability scan.
- E2E tests for protected flows before release.

Release requirements:

- Signed release artifacts.
- Checksums.
- Migration notes.
- Agent compatibility matrix.
- Rollback instructions.

## Observability and Operations

Panel must expose:

- Health endpoint.
- Readiness endpoint.
- Version endpoint.
- Agent heartbeat status.
- Queue depth.
- Failed job count.
- Service health summary.
- Audit log search.

Operators must be able to answer:

- What changed?
- Who changed it?
- Which server was affected?
- Did the job complete?
- What config file changed?
- How do we roll it back?

## Boundaries

### Always Do

- Validate all external input.
- Authorize all resource access.
- Route privileged work through agent jobs.
- Record audit events for privileged and security-sensitive actions.
- Validate service config before reload.
- Snapshot managed config before modification.
- Redact secrets in logs.
- Keep docs and ADRs updated for architectural decisions.

### Ask First

- Adding a new root-level agent capability.
- Changing authentication or authorization model.
- Adding a new external provider integration.
- Adding billing or license enforcement.
- Supporting a new OS family.
- Changing public API contracts.
- Introducing multi-node behavior.

### Never Do

- Execute arbitrary shell commands from user input.
- Store plaintext passwords or secrets.
- Trust client-side validation.
- Expose internal stack traces to users.
- Modify files outside allowed account roots without explicit system-level action.
- Reload or restart services without config validation where validation exists.
- Hide failed jobs from operators.

## Architectural Decisions To Record

ADRs should be written for:

- Primary web/backend stack.
- Agent implementation language.
- Panel database choice.
- API style and versioning.
- Job queue technology.
- Secret storage approach.
- Plugin architecture.
- First supported OS targets.
- Packaging and installer strategy.
- Web server adapter priority.

## Initial Decisions

1. Main web application: Go, optimized for low-resource VPS deployments.
2. First UI: custom operational dashboard tailored to hosting workflows.
3. Agent: Go.
4. Panel database: PostgreSQL by default and minimum supported database.
5. Email hosting: included in product scope and first public release plan when reliability requirements are met.
6. Container hosting: not included; use native OS services and runtimes.
7. Product model: open source initially, with a possible commercial path later.
8. License, billing, and reseller features: design from the beginning so future commercial/provider workflows are not blocked.

## Success Criteria

- The product has a documented modular architecture.
- Core resources and permissions are defined.
- Privileged system operations are isolated behind an agent.
- Service adapters can be added without rewriting the panel core.
- API contracts are stable and versioned.
- Full hosting feature domains are accounted for.
- Security, audit, backup, and rollback are first-class requirements.
- The product can evolve from single-server install to multi-server control plane.
