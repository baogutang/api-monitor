# Update and Release

API Monitor supports the same deployment pattern as many self-hosted relay
projects:

- GitHub Actions builds release tarballs for Linux and macOS.
- GitHub Actions builds and pushes a multi-arch Docker image to GHCR.
- The running app can check GitHub Releases from the Settings page/API.
- Self-update is disabled by default and must be explicitly configured.

## Required Repository Setting

Set this environment variable in deployment:

```bash
GITHUB_REPO=baogutang/api-monitor
```

The backend calls:

```http
POST /api/v1/version/check
```

and compares the current build version with the latest GitHub Release.

## Docker Deployment Update

Use the source-build compose file while developing:

```bash
docker compose up --build -d
```

Use the release compose file for a deployed GHCR image:

```bash
export API_MONITOR_IMAGE=ghcr.io/baogutang/api-monitor:latest
export GITHUB_REPO=baogutang/api-monitor
docker compose -f docker-compose.release.yml up -d
```

For Docker Compose, the safest update command is controlled by the operator:

```bash
ENABLE_SELF_UPDATE=true
UPDATE_COMMAND='docker compose -f docker-compose.release.yml pull api worker && docker compose -f docker-compose.release.yml up -d'
```

This only works when the container has access to Docker/Compose in the deployment
environment. Do not enable it on untrusted networks.

Recommended production pattern:

- Keep `ENABLE_SELF_UPDATE=false`.
- Let the page show the latest version and release link.
- Use Watchtower, Portainer, or your own deployment pipeline to apply the update.

## Binary/Tarball Deployment Update

For tarball installs, provide a wrapper script and point `UPDATE_COMMAND` to it:

```bash
ENABLE_SELF_UPDATE=true
UPDATE_COMMAND='/opt/api-monitor/update.sh'
```

The script should download the latest release asset, stop the service, replace
the binary/assets, run migrations, and restart the service.

## Creating a Release

```bash
git tag v1.0.0
git push origin v1.0.0
```

The workflow will publish:

- `api-monitor_<version>_linux_amd64.tar.gz`
- `api-monitor_<version>_linux_arm64.tar.gz`
- `api-monitor_<version>_darwin_amd64.tar.gz`
- `api-monitor_<version>_darwin_arm64.tar.gz`
- `ghcr.io/<owner>/<repo>:<version>`
- `ghcr.io/<owner>/<repo>:latest`
