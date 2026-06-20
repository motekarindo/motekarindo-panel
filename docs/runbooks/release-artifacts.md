# Release Artifacts

This runbook describes how to build Motekar Panel release artifacts for Ubuntu 24.04 amd64.

## Artifacts

The release pipeline produces:

- `motekarctl-linux-amd64`
- `motekarctl-linux-amd64.sha256`
- `motekar-panel-linux-amd64`
- `motekar-panel-linux-amd64.sha256`
- `motekar-agent-linux-amd64`
- `motekar-agent-linux-amd64.sha256`
- `install-ubuntu-24.04-amd64.sh`
- `install-ubuntu-24.04-amd64.sh.sha256`

## Local Build

Build artifacts locally:

```bash
make release-artifacts
```

Verify release artifact generation and checksums:

```bash
make test-release-artifacts
```

Artifacts are written to `dist/`.

## GitHub Release

Create and push a tag:

```bash
git tag v0.1.0-alpha.1
git push origin v0.1.0-alpha.1
```

The `Release` workflow builds artifacts and publishes them to GitHub Releases.

## End-User Installer Flow

After a release exists, operators can run:

```bash
curl -fsSLO https://github.com/motekarindo/motekarindo-panel/releases/latest/download/install-ubuntu-24.04-amd64.sh
chmod +x install-ubuntu-24.04-amd64.sh
./install-ubuntu-24.04-amd64.sh --dry-run --profile single-user --web-server nginx
```

The bootstrapper downloads `motekarctl-linux-amd64` and verifies `motekarctl-linux-amd64.sha256` by default.

`--apply` is intentionally refused until actual installer apply support exists.
