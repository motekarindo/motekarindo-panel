# System Test Safety Runbook

Motekar Panel must not run destructive OS-module tests on the developer or operator host machine.

## Safe on Host

These are allowed on the host:

```bash
make test
make build
make fmt
```

Allowed test categories:

- Unit tests.
- Golden file tests.
- Config rendering tests.
- Validation tests.
- HTTP handler tests.
- Non-privileged integration tests.

## Not Safe on Host

Do not run tests on the host if they do any of this:

- Install packages with `apt`, `dnf`, or `yum`.
- Write to `/etc`, `/var`, `/usr`, `/home`, or systemd directories.
- Run `systemctl`.
- Run `useradd`, `groupadd`, `usermod`, or quota tools.
- Reload or restart Nginx, Apache, PHP-FPM, MariaDB, PostgreSQL, Postfix, Dovecot, DNS, firewall, or security services.
- Modify firewall rules.
- Modify SELinux state or policy.
- Create real hosting accounts or Linux users.

## Required Pattern for OS Modules

Every OS module should be tested in this order:

1. Unit tests for validation and decision logic.
2. Golden file tests for generated config.
3. Dry-run plan tests for intended filesystem/service actions.
4. Disposable Ubuntu 24.04 system tests.
5. Staging server tests before release.

## Required System Test Guard

System tests must refuse to run unless this environment variable is set:

```bash
MOTEKAR_TEST_ALLOW_SYSTEM=1
```

The test runner must also verify that it is inside an approved disposable environment.

## Approved Environments

Use one of these:

- Disposable Ubuntu 24.04 VM.
- Disposable Multipass Ubuntu 24.04 instance.
- Disposable LXD Ubuntu 24.04 container when container behavior is sufficient.
- Dedicated throwaway VPS.
- CI runner dedicated to destructive system tests.

## Example: Nginx Module

Host-safe tests:

- Validate domain names.
- Render Nginx vhost config.
- Compare vhost config to golden file.
- Build dry-run plan:
  - write config file
  - run `nginx -t`
  - enable site
  - reload Nginx

Disposable system tests:

- Install Nginx.
- Write test vhost config.
- Run `nginx -t`.
- Reload Nginx.
- Curl local test domain.
- Tear down VM/container.

## Example: PHP Switch

Host-safe tests:

- Detect valid PHP version strings.
- Render PHP-FPM pool config.
- Compare pool config to golden file.
- Build dry-run plan for switching one site.

Disposable system tests:

- Install PHP-FPM versions.
- Create test account user.
- Create two test sites.
- Assign different PHP versions.
- Curl both sites and verify `PHP_VERSION`.
- Tear down VM/container.

