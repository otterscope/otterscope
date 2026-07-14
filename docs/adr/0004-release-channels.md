# ADR-0004: Release channels and packaging

Date: 2026-07-14 · Status: accepted

## Context
Distribution is the product's whole thesis (ADR-0001): install must be one command everywhere our users live. The self-hosted community expects an official Docker image and a docker-compose example; developers expect binaries, Homebrew, and `go install`. Because the binary is static and CGO-free, every channel below is buildable from one GitHub Actions release pipeline with no signing infrastructure required at this stage.

## Decision
Tag-triggered (`v*`) GoReleaser pipeline in GitHub Actions publishing, in priority order:

1. **GitHub Releases** — static binaries + checksums for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64.
2. **Docker image on GHCR** — `ghcr.io/otterscope/otterscope`, multi-arch (amd64/arm64), `FROM scratch` + binary + CA certs; tags `latest`, `X.Y.Z`, `X.Y`. A `docker-compose.yml` example lives in the repo root.
3. **Homebrew tap** — `otterscope/homebrew-tap`, formula auto-bumped by GoReleaser.
4. **deb/rpm via nfpm** (inside GoReleaser) attached to GitHub Releases; a proper apt repo only if demand appears.
5. **`go install github.com/otterscope/otterscope/cmd/otterscope@latest`** — works implicitly; keep the module path stable.

Deferred until users ask: Windows signing/Scoop/winget, apt/yum hosted repos, Nix, Helm chart (a plain K8s manifest example ships in docs instead).

## Consequences
- Release cost per version is one git tag; everything else is CI.
- The Docker image must stay `FROM scratch`-compatible: no runtime file dependencies beyond the binary and the SQLite data dir — this constrains features the same way ADR-0001 does.
- Version is injected via `-ldflags -X main.version=`; `otterscope version` must always report accurately.
