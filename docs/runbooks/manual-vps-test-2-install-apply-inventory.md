# Manual VPS Test 2: Install Apply and Inventory

This test verifies the actual installer apply path and the inventory dashboard on a disposable Ubuntu 24.04 VPS.

Do not use an important server. Use a fresh VPS that can be rebuilt.

## Scope

This test is allowed to:

- Run preflight and an installer dry-run plan.
- Run `motekarctl install apply` against a disposable PostgreSQL database.
- Persist and verify the immutable `settings.webserver` server setting and its audit events.
- Run the panel and agent and inspect the server inventory dashboard.

This test must not:

- Actually install PostgreSQL, Nginx/Apache, or systemd services (installer executors for those do not exist yet).
- Point at production or important data.

## VPS Requirement

- Ubuntu 24.04 LTS, amd64, root or sudo shell, fresh and rebuildable.
- This test assumes a 2 vCPU / 2 GB RAM / 30 GB disk VPS, so the `single-user` profile is used. A 2 GB VPS reports about 1967 MB RAM, which is below the 2048 MB `shared-hosting` minimum, so `shared-hosting` is expected to block at preflight.

## Setup

Build the binaries and copy them to the VPS, or use the release flow. From the repository checkout:

```bash
make build
scp .cache/motekarctl-linux-amd64 2>/dev/null || scp dist/motekarctl-linux-amd64 root@VPS:/tmp/motekarctl 2>/dev/null
```

If no release binary exists yet, build `motekarctl`, `motekar-panel`, and `motekar-agent` for linux/amd64 and copy them over:

```bash
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
- `--apply` is refused with `--apply is not available yet; this bootstrapper is dry-run only`.
- Preflight reports PASS for os/cpu/disk/root/systemd/ports and either PASS or WARN for memory and swap (single-user minimums: 960 MB RAM and 1 GB swap; recommendation is 2 GB).
- Plan prints `WOULD_CHANGE` actions and ends with `No changes were made.`

## Step 2: Disposable PostgreSQL

Install PostgreSQL on the VPS or use an existing disposable instance, then create a test database and user. For a quick local install:

```bash
apt-get update && apt-get install -y postgresql
sudo -u postgres psql -c "CREATE USER motekar WITH PASSWORD 'motekar';"
sudo -u postgres psql -c "CREATE DATABASE motekar_vps_test OWNER motekar;"
```

Migrate the schema manually (the `database.migrate` install action is not implemented yet):

```bash
export MOTEKAR_DATABASE_URL="postgres://motekar:motekar@127.0.0.1:5432/motekar_vps_test?sslmode=disable"
/tmp/motekar-panel migrate up
```

## Step 3: Installer Apply (Task 4.3)

Run the actual apply path with the same database URL:

```bash
/tmp/motekarctl install apply \
  --profile single-user \
  --web-server nginx \
  --postgresql install \
  --database-url "$MOTEKAR_DATABASE_URL"
```

Expected:

- Output lists 5 skipped actions: `preflight.verify`, `postgresql.install`, `webserver.install`, `database.migrate`, `systemd.services`.
- Output ends with `web_server: nginx` and the persistence confirmation.
- Database now has a `server_settings` row with immutable `web_server` = `nginx`, and an audit event `settings.web_server.selected`.

Verify the setting and audit rows:

```bash
sudo -u postgres psql -d motekar_vps_test -c "SELECT key, value, is_immutable FROM server_settings WHERE key = 'web_server';"
sudo -u postgres psql -d motekar_vps_test -c "SELECT action, metadata FROM audit_events ORDER BY created_at DESC LIMIT 3;"
```

Test immutability: a second apply with a different web server must fail and be audited:

```bash
/tmp/motekarctl install apply \
  --profile single-user \
  --web-server apache \
  --postgresql install \
  --database-url "$MOTEKAR_DATABASE_URL"
```

Expected: apply fails (`web server is already selected: nginx`) and the database records `settings.web_server.change_denied` with metadata `{"value":"apache","current":"nginx"}`.

Test idempotency: applying the same value again is currently expected to fail too, because the setting is already immutable and `Select` rejects any subsequent write regardless of value. Verify the actual behavior and record it:

```bash
/tmp/motekarctl install apply \
  --profile single-user \
  --web-server nginx \
  --postgresql install \
  --database-url "$MOTEKAR_DATABASE_URL"
```

Expected: apply fails (`web server is already selected: nginx`) and the database records `settings.web_server.change_denied`. If the same-value apply instead succeeds, note that divergence from `internal/settings/webserver.go` in the verification record.

## Step 4: Panel, Agent, and Inventory (Task 4.4)

Start the agent:

```bash
mkdir -p /var/run/motekar-panel
MOTEKAR_AGENT_SOCKET=/var/run/motekar-panel/agent.sock /tmp/motekar-agent serve &
```

Verify the agent reports `server.inventory`:

```bash
curl --unix-socket /var/run/motekar-panel/agent.sock http://agent/capabilities
curl --unix-socket /var/run/motekar-panel/agent.sock \
  -X POST http://agent/actions/server.inventory \
  -H 'Content-Type: application/json' -d '{"payload":{}}'
```

Start the panel:

```bash
MOTEKAR_DATABASE_URL="$MOTEKAR_DATABASE_URL" \
MOTEKAR_AGENT_SOCKET=/var/run/motekar-panel/agent.sock \
MOTEKAR_PANEL_ADDR=127.0.0.1:8080 \
/tmp/motekar-panel serve &
```

Bootstrap the first admin:

```bash
printf '%s\n' 'vps-test-password-123' | \
  MOTEKAR_DATABASE_URL="$MOTEKAR_DATABASE_URL" /tmp/motekar-panel bootstrap admin \
    --email owner@example.com --display-name "Owner" --password-stdin
```

Check readiness, log in, and open the inventory dashboard:

```bash
curl http://127.0.0.1:8080/readyz
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
- Web server card shows `nginx` from the immutable setting persisted in Step 3.
- A non-admin account (or no session) gets `403` on `/inventory`.

Stop the agent, then reload `/inventory`:

- Page still returns `200` and renders `Agent unavailable` with a clear message.

## Step 5: Cleanup

- Stop the panel and agent processes.
- Destroy the test database and the VPS.

## Verification Record

- Date / VPS spec / profile / commit used: fill in after the run.