# Current Sprint: README Rewrite + Container Registry for v0.5.0

## Goal

1. Rewrite README.md as a clean deployment-only guide (no developer content)
2. Set up GitHub Actions to publish prebuilt images to GitHub Container Registry
3. Provide a standalone docker-compose.yml so users never need to build anything

## Part 1: GitHub Container Registry

### New file: `.github/workflows/publish.yml`

Workflow that builds and pushes the Docker image to `ghcr.io/wan-ninjas/amityvox`.

**Triggers:**
- Push to `main` branch (tags as `latest`)
- Push of version tags `v*` (tags as version, e.g. `v0.5.0` and `0.5.0`)

**Steps:**
1. Checkout code
2. Set up Docker Buildx (for multi-platform)
3. Login to ghcr.io using `GITHUB_TOKEN`
4. Extract metadata (tags + labels from git ref)
5. Build + push multi-platform image (linux/amd64, linux/arm64)
6. Pass build args: VERSION, COMMIT, BUILD_DATE

**Key actions used:**
- `docker/setup-buildx-action`
- `docker/login-action` (registry: ghcr.io, username: `${{ github.actor }}`, password: `${{ secrets.GITHUB_TOKEN }}`)
- `docker/metadata-action` (generates tags from git ref)
- `docker/build-push-action` (multi-platform, push: true)

**Package visibility:** Set to public in repo settings (Settings > Packages) or via workflow label.

### New file: `docker_deploy/docker-compose.yml`

A standalone, user-facing compose file that uses prebuilt images. No `build:` directives, no `../../` paths.

```yaml
services:
  amityvox:
    image: ghcr.io/wan-ninjas/amityvox:latest
    # ... same env/depends as current, but paths simplified

  web-init:
    image: ghcr.io/wan-ninjas/amityvox:latest
    entrypoint: ["sh", "-c", "cp -r /srv/web/* /srv/public/"]
    # Frontend is baked into the image at /srv/web

  # All other services (postgresql, nats, dragonfly, etc.) stay as-is
  # since they use public images already
```

Users download this single file + `.env.example` and run `docker compose up -d`.

**The existing `deploy/docker/docker-compose.yml` stays** for development (builds from source).

### Config files

The compose also needs supporting config files. Options:
- Inline defaults via environment variables (preferred — fewer files to download)
- Or bundle configs into the image (garage.toml, livekit.yaml, Caddyfile)

Check which configs are mounted as volumes and decide if they can be embedded or provided as env vars.

## Part 2: README Rewrite

### Structure

1. **Header** — Name, one-liner, version badge, license badge, Discord badge
2. **Links** — Discord | Live Instance | Docs
3. **Features** — Single compact bullet list (no sub-sections)
4. **Quick Start**
   - Prerequisites (Docker + Compose, domain, 2GB RAM)
   - Option A: One-command deploy (curl compose + env, edit, up)
   - Option B: Clone repo and `docker compose up`
5. **Initial Setup**
   - Web setup wizard (auto on first launch, locked after completion)
   - Create admin user via CLI
6. **Configuration** — `.env` reference, key variables, TLS/domain setup
7. **Services** — Table of containers
8. **CLI Reference** — Brief subcommand list
9. **Backup & Restore** — One-liners
10. **Community** — Discord, instance, license

### What to Remove

- Local Development section
- Testing section
- Project Structure tree
- API Overview / endpoint listing
- Contributing section
- Tech Stack detailed table
- Desktop & Mobile section (Tauri/Capacitor)

### Key Info

- **Version**: v0.5.0
- **Discord**: https://discord.gg/VvxgUpF3uQ
- **Live instance**: https://amityvox.chat/invite/b38a4701f16f
- **License**: AGPL-3.0

### Setup Wizard Notes

- `GET /api/v1/admin/setup/status` — returns `{completed, instance_name, instance_id}`
- `POST /api/v1/admin/setup/complete` — sets instance name, description, domain, registration mode, federation mode
- **Gating**: Before setup completion, anyone can access. After `instance_settings.setup_completed = 'true'`, requires admin auth.
- Admin user creation is separate (CLI only): `docker exec amityvox amityvox admin create-user <username> <email> <password>`

## Verification

- [x] `publish.yml` builds and pushes on main/tags
- [x] Multi-platform: linux/amd64 + linux/arm64
- [x] Root `docker-compose.yml` uses `image:` not `build:`
- [x] Users can deploy with just compose file + .env (no clone needed)
- [x] README says v0.5.0
- [x] Discord and instance links present
- [x] No developer instructions in README
- [x] Setup wizard + admin creation documented
- [x] Existing `deploy/docker/docker-compose.yml` unchanged (dev use)
