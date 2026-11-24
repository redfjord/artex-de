# FailSafe ARTEX — upstream provenance

## Source

| Field | Value |
|---|---|
| Upstream project | `RuoJi6/ARTEX` |
| Upstream tag | `v0.3.15` |
| Tag commit | `a91b27feb7168c1486f827559bc48a589a920012` |
| Imported | 2026-11-10 |
| License | AGPL-3.0 (`LICENSE`) |
| Original author | Autumn-27 / ARTEX contributors |

`RuoJi6/ARTEX` is the best available source base after the original upstream
repository became unavailable. The `v0.3.15` tag was chosen because it predates
the `v0.3.16` prebuilt image that carried the `verifyCaptcha` implant, and its
Go sources match the tree that was audited on 2026-11-04/10.

The self-update constant currently assumes the public repository will be
`failsafe-security/artex`. If the final GitHub organization or repository name
differs, change it in `selfupdate/github.go` and `selfupdate/github_test.go`,
then rebuild and record the new binary hash. Do not make the update source
runtime-configurable: that would turn a writable setting into a code-execution
path.

## What is unchanged

All Go source files, `go.mod`, `go.sum`, `start.sh`, and `skills/` are
byte-identical to the audited `v0.3.15` tree, except for the deliberate
changes below. The web UI source is unchanged.

## What changed

1. **`selfupdate/github.go`** — `const Repo` changed from `RuoJi6/ARTEX` to
   `failsafe-security/artex`. This points the self-update release check at this
   fork instead of upstream. The Docker build embeds a `-failsafe.N` version
   suffix, which disables the one-click updater, so this is defense in depth.
2. **`selfupdate/github_test.go`** — test-only updates so the expected URL,
   error string, and fixture URLs match the new `Repo` constant. No production
   behavior changes.
3. **Distribution files removed** — the upstream `Dockerfile`,
   `docker-compose.yml`, `docker-compose.bench.yml`, `install.sh`, `update.sh`,
   `bench/`, `.env.example`, and `.github/workflows/release.yml` were removed
   so nobody accidentally pulls a prebuilt ARTEX image, publishes one from this
   fork, or runs an unaudited installer from this repository.
4. **`README.md`** — replaced with the FailSafe build/run instructions.
   Upstream's README is preserved as `README.upstream.md`.
5. **Added** — `Dockerfile.safe`, `docker-compose.safe.yml`, `.env.safe.example`,
   `SECURITY.md`, `UPSTREAM.md`, `Makefile`, `.dockerignore`, `.gitignore`, and
   this repository's audit reports under `audit/`.
6. **Runtime capability parity** — `Dockerfile.safe` installs the same browser
   stack as the upstream Dockerfile: Node 22, `@playwright/mcp`,
   `@playwright/cli`, Playwright, and Chromium. It runs as root like upstream so
   nmap raw scans and runtime tool installation work. The deployment differences
   are loopback-only console binding, an unpublished proxy, and
   `no-new-privileges`.
7. **`db/db.go`** — the seeded browser MCP launches the pinned `playwright-mcp`
   binary with `["--headless","--no-sandbox"]` instead of
   `npx @playwright/mcp`. This avoids a registry lookup, an accidental
   `@latest` download, and Chromium startup failures as root. The MCP is still
   seeded disabled and remains an admin-enabled feature.

8. **Reviewed prompt-cache backport** — the per-intent artifact path and task
   block are placed after the stable worker prompt and deferred-tool block.
   Deferred host-tool names are sorted before rendering. This backports the
   small cache fix from PR #10 without importing v0.3.30, changing asset queries,
   or weakening deployment isolation. Custom prompt variables still render on
   each run. Deterministic inputs are necessary for a stable prefix.

9. **Static UI delivery** — `server/static_files.go` compresses the embedded
   frontend (Brotli, gzip fallback), emits content-hash ETags, marks the
   content-hashed `/_next/static/` tree immutable, and revalidates everything
   else. A missing build asset now returns 404 instead of the SPA document.
   Upstream served the `embed.FS` through `http.ServeFileFS`, which sent the
   multi-megabyte bundle uncompressed with no validator; direct `:8787`
   deployments (our compose file) re-downloaded it on every navigation and
   stalled behind the browser's six-connections-per-host limit, which presents
   as the UI timing out after every click. Reproduced with headless Chrome;
   covered by unit tests and `ci/e2e_binary.py`.

10. **English interface and documentation** — the web console's user-visible
    strings and comments, agent-facing skills, shell/script comments, and
    project documentation are translated to English. Backend-generated log and
    error messages remain in their original language in this release; the UI
    translates its own labels, states, and validation text. Protocol values,
    parser tokens, and API field names were not renamed.

The online-update and API-recon changes below are additional deliberate
differences. No v0.3.30 application upgrade is included.

## The incident this fork responds to

On 2026-11-03/04, public analysis found that prebuilt Docker images published
under `tomicdd/artex` (and copies under `congdonglinux/artex`) contained an
unauthenticated command-execution backdoor in `server/auth.go`:

- a hidden `server.verifyCaptcha` function called
  `exec.Command("sh", "-c", input)`;
- `authLogin` invoked it when the `captcha` JSON field began with a secret
  marker, before checking credentials;
- the command output was returned in the login response as `{"captcha": ...}`.

The GitHub source did not contain the implant; it was image-only. This fork
therefore **builds from source and never pulls an ARTEX image**. The exact base
images used by the build are pinned by digest in `Dockerfile.safe`.

See:

- [`audit/artex-v0.3.15-audit.md`](audit/artex-v0.3.15-audit.md) — this tag
- [`SECURITY.md`](SECURITY.md) — deployment guidance and residual risk

## Update policy

1. Do not merge upstream changes blindly. Fetch the tag, diff it against this
   tree, and review every change to `agent/`, `server/`, `cmd/`, `skills/`,
   `web/`, `start.sh`, `go.mod`, and `go.sum`.
2. Rebuild with `make build`, then run `make verify` to check the known
   implant indicators and record the new binary hash.
3. Update the audit report and the pinned base-image digests in
   `Dockerfile.safe` deliberately.
4. Never add a `docker-compose.yml` that references `tomicdd/artex`,
   `congdonglinux/artex`, or any other prebuilt ARTEX image.

## Upstream links

- Source: https://github.com/RuoJi6/ARTEX
- License: https://www.gnu.org/licenses/agpl-3.0.html

## Local online update hardening

Online release checks and downloads are disabled by `selfupdate.Enabled = false`, independently of the version string. The fixed repository identifier and derived URL remain for source clarity, but FetchLatest and Stage refuse before network access. Update API check reports disabled; apply and rollback return 403. Local staged-file bootstrap recovery remains available. Deployment updates require a reviewed source rebuild. Earlier descriptions of version-suffix-only disabling are historical.


## Local API recon supply chain hardening

The API-recon runtime lockfile, module loader, and dependency setup instructions deliberately differ from upstream. Puppeteer is pinned to 25.17.0, fetched from the official npm registry with integrity checks and lifecycle scripts disabled, and preinstalled at image build time. The image uses an absolute module path; missing image dependencies fail without a runtime install. General skill translations are excluded. The previous skills-tree and image hashes remain historical and must be replaced with results from the final release build before promotion.
