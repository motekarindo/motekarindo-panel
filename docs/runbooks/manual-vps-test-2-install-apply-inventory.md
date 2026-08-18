# Manual VPS Test 2: One-Shot Install Apply and Inventory

This test verifies the full installer apply path (PostgreSQL, web server, binaries, systemd services, database migrate, first admin) and the inventory dashboard on a disposable Ubuntu 24.04 VPS.

Do not use an important server. Use a fresh VPS that can be rebuilt.

## Scope

This test is allowed to:

- Run preflight and an installer dry-run plan.
- Run `--apply` through the end-user bootstrapper, which installs PostgreSQL, Nginx/Apache, the panel/agent binaries, systemd services, runs migrations, and bootstraps the first admin.
- Persist and verify the immutable `settings.webserver` server setting and its audit events.
- Run the panel and agent and inspect the server inventory dashboard.

This test must not:

- Run on production or important data.

## VPS Requirement

- Ubuntu 24.04 LTS, amd64, root or sudo shell, fresh and rebuildable.
- This test assumes a 2 vCPU / 2 GB RAM / 30 GB disk VPS, so the `single-user` profile is used. A 2 GB VPS reports about 1967 MB RAM, which is below the 2048 MB `shared-hosting` minimum, so `shared-hosting` is expected to block at preflight.

## Setup

For a release-based run, no setup is needed beyond the bootstrapper itself (it downloads all three binaries and verifies checksums). For a development run, build the three binaries for linux/amd64 and copy them to the VPS:

```bash
make build
GOOS=linux GOARCH=amd64 go build -o /tmp/motekarctl ./cmd/motekarctl
GOOS=linux GOARCH=amd64 go build -o /tmp/motekar-panel ./cmd/motekar-panel
GOOS=linux GOARCH=amd64 go build -o /tmp/motekar-agent ./cmd/motekar-agent
```

## Step 1: Bootstrapper Dry Run (read-only)

Use the end-user installer script from GitHub Releases, or point it at a local binary for development:

```bash
curl -fsSLO https://github.com/motekarindo/motekarindo-panel/releases/latest/download/install-ubuntu-24.04-amd64.sh
chmod +x install-ubuntu-24.04-amd64.sh

./install-ubuntu-24.04-amd64.sh --dry-run \
  --profile single-user \
  --web-server nginx \
  --postgresql install
```

Expected:

- Script runs as root and accepts Ubuntu 24.04 amd64.
- Download verifies the checksum (`motekarctl: OK`) when pulling from Releases.
- Preflight reports PASS for os/cpu/disk/root/systemd/ports and either PASS or WARN for memory and swap (single-user minimums: 960 MB RAM and 1 GB swap; recommendation is 2 GB).
- Plan prints `WOULD_CHANGE` actions and ends with `No changes were made.`
- No system packages are installed and no files are written.

## Step 2: One-Shot Apply (Tasks 4.0-4.3)

Run the full install through the bootstrapper. This installs PostgreSQL, the web server, the panel/agent binaries, writes systemd services, runs the embedded migrations, and bootstraps the first admin. The installer wraps itself in a tmux session when running on an interactive terminal, so the install survives SSH disconnects (`tmux attach -t motekar-install` to reattach).

Release run (downloads all binaries and prompts for admin credentials):

```bash
./install-ubuntu-24.04-amd64.sh --apply \
  --profile single-user \
  --web-server nginx \
  --postgresql install
```

Development run (uses locally built binaries from a directory, non-interactive):

```bash
mkdir -p /tmp/devbin
cp /tmp/motekarctl /tmp/devbin/motekarctl-linux-amd64
cp /tmp/motekar-panel /tmp/devbin/motekar-panel-linux-amd64
cp /tmp/motekar-agent /tmp/devbin/motekar-agent-linux-amd64

./install-ubuntu-24.04-amd64.sh --apply \
  --profile single-user \
  --web-server nginx \
  --postgresql install \
  --local-binary-dir /tmp/devbin \
  --bin-dir /usr/local/bin \
  --admin-email owner@example.com \
  --admin-display-name "Owner" \
  --admin-password "vps-test-password-123"
```

Expected:

- The three binaries are installed under `/usr/local/bin`.
- The install prints `running <action> ...` before and `done` after each plan action, and apt-get output streams live to the terminal instead of going silent.
- `apt-get` installs PostgreSQL and Nginx; both are enabled.
- PostgreSQL role `motekar` and database `motekar_panel` are created with a generated password.
- Environment files are written to `/etc/motekar-panel/panel.env` and `agent.env` (mode 0600).
- Unit files `motekar-panel.service` and `motekar-agent.service` are written to `/etc/systemd/system` and both are enabled and started.
- The embedded migrations run and create the schema.
- The first admin `owner@example.com` is created.
- Output ends with `Motekar Panel installed.` and the panel URL.
- No `skipped ... action(s) not yet supported` line is printed.

Verify installed state:

```bash
systemctl status motekar-panel motekar-agent --no-pager
curl http://127.0.0.1:8080/readyz
sudo -u postgres psql -d motekar_panel -c "SELECT key, value, is_immutable FROM server_settings WHERE key = 'web_server';"
sudo -u postgres psql -d motekar_panel -c "SELECT action, metadata FROM audit_events ORDER BY created_at DESC LIMIT 3;"
```

Expected:

- Both services are `active (running)`.
- `/readyz` returns ready.
- `server_settings` has an immutable `web_server` = `nginx`, and an audit event `settings.web_server.selected`.

If the SSH connection drops mid-install, reconnect and attach to the tmux session to see the install continue:

```bash
tmux attach -t motekar-install
```

## Step 3: Immutability and Idempotency

Run `motekarctl install apply` directly with a different web server:

```bash
/usr/local/bin/motekarctl install apply \
  --profile single-user \
  --web-server apache \
  --postgresql install \
  --admin-email owner@example.com \
  --admin-password-stdin <<< 'vps-test-password-123'
```

Expected: apply fails (`web server is already selected: nginx`) and the database records `settings.web_server.change_denied` with metadata `{"value":"apache","current":"nginx"}`.

Test idempotency: applying the same value again is currently expected to fail too, because the setting is already immutable and `Select` rejects any subsequent write regardless of value. Verify the actual behavior and record it:

```bash
/usr/local/bin/motekarctl install apply \
  --profile single-user \
  --web-server nginx \
  --postgresql install \
  --admin-email owner@example.com \
  --admin-password-stdin <<< 'vps-test-password-123'
```

Expected: apply fails (`web server is already selected: nginx`) and the database records `settings.web_server.change_denied`. If the same-value apply instead succeeds, note that divergence from `internal/settings/webserver.go` in the verification record.

## Step 4: Panel, Agent, and Inventory (Task 4.4)

Both services are already running from Step 2. Verify the agent reports `server.inventory`:

```bash
curl --unix-socket /run/motekar-panel/agent.sock http://agent/capabilities
curl --unix-socket /run/motekar-panel/agent.sock \
  -X POST http://agent/actions/server.inventory \
  -H 'Content-Type: application/json' -d '{"payload":{}}'
```

Log in and open the inventory dashboard:

```bash
curl -i -c /tmp/motekar-cookies.txt \
  -H 'Origin: http://127.0.0.1:8080' \
  --data-urlencode 'email=owner@example.com' \
  --data-urlencode 'password=vps-test-password-123' \
  http://127.0.0.1:8080/login
curl -b /tmp/motekar-cookies.txt http://127.0.0.1:8080/inventory
```

Expected on the Ubuntu host:

- `/readyz` returns ready.
- `/inventory` shows agent `online` with real values: OS `ubuntu 24.04`, kernel, CPU cores, RAM, swap, disk free, load, uptime, interface addresses, and systemd service units.
- Web server card shows `nginx` from the immutable setting persisted in Step 2.
- A non-admin account (or no session) gets `403` on `/inventory`.

Stop the agent service, then reload `/inventory`:

```bash
systemctl stop motekar-agent
curl -b /tmp/motekar-cookies.txt http://127.0.0.1:8080/inventory
```

Expected:

- Page still returns `200` and renders `Agent unavailable` with a clear message.
- Restart the agent afterwards: `systemctl start motekar-agent`.

## Step 5: Cleanup

- Destroy the test database and the VPS.

## Verification Record

- Date / VPS spec / profile / commit used: fill in after the run.