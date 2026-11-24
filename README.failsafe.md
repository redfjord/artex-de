# FailSafe ARTEX

<p align="center">
  <img src="assets/artex-failsafe-header.png" alt="ARTEX — a source-built FailSafe distribution" width="100%">
</p>

A source-built, locally bound distribution of [ARTEX](https://github.com/RuoJi6/ARTEX),
an open-source multi-agent security testing harness — with an English interface,
a hardened supply chain, and deployment controls.

This repository exists because the prebuilt `tomicdd/artex` Docker image was
found to contain an unauthenticated command-execution backdoor in November 2026.
This build is compiled from the audited `v0.3.15` source tag and keeps upstream's
full runtime capabilities, including Node 22, Playwright, Chromium, and nmap.
The compose file binds the console to localhost and does not publish the
recording proxy.

> **Authorized testing only.** ARTEX runs an autonomous agent that can execute
> shell commands and send network traffic. Use it only against systems you own
> or have explicit written permission to test.

## Maintained by FailSafe Cyber Frontier Labs

This distribution is maintained by **FailSafe Cyber Frontier Labs**, the team
behind GlassBreak and FailSafe's managed security-testing runtime. We review
upstream changes, maintain the hardened build, and operate ARTEX for teams that
need the capability without building the guardrails themselves.

- General and security questions: <hello@getfailsafe.com>
- Managed ARTEX runs and cyber-model access: <hello@getfailsafe.com>
- Website: <https://getfailsafe.com>

## Recommended models

ARTEX is an agentic harness: the model plans, calls tools, and reasons about
targets across many turns. For authorized security work we recommend a
**cyber-specialized model** — one trained or fine-tuned for security tasks
(cyber-domain SFT) with minimal refusal behavior, rather than a general-purpose
chat model. General consumer models routinely refuse reconnaissance, payload
construction, or exploitation reasoning even when the engagement is authorized,
which turns a legitimate test into a stream of blocked tool calls and wasted
tokens.

**GlassBreak** is FailSafe's cyber-grade model, built for this workload. It is
what we run when we operate ARTEX ourselves, and it is what the managed runs
use. If you do not already have a cyber model you can use, contact
<hello@getfailsafe.com> to arrange a managed run or rent model access for
runs on infrastructure you control.

## Features

- **Autonomous multi-agent testing.** A goals agent decomposes the objective, an
  event-driven planner explores, and concurrent workers execute intents. New
  assets, facts, findings, and results feed back into planning. A main
  conversation agent lets a human steer mid-run.
- **Real execution capabilities.** Shell and file tools, browser automation
  through Playwright MCP/CLI and bundled Chromium, skills, MCP servers, custom
  Python tools, DNS/network recon, and raw SYN scans with `nmap`.
- **Evidence-backed findings.** Findings keep task/asset lineage and recorded
  HTTP evidence, generate reports, and enter a dedicated retest workflow with
  `reproduced`, `fixed`, or `inconclusive` verdicts.
- **Traffic capture.** A recording proxy indexes HTTP exchanges, bodies, and
  archives; evidence binds to findings; archives export and reclaim safely.
- **Operational controls.** Admin authentication, configurable tool
  intercept/allow/deny rules, optional model-judge approvals, pause/resume,
  queueing, cancellation, deadlines, restart recovery, and task isolation.
- **Full English interface and documentation.** The console, agent skills, and
  project documentation are in English for international teams.
- **Fast static delivery.** The embedded web console is served with Brotli/gzip
  compression, content-hash ETags, and immutable caching, so the UI stays
  responsive when the binary is reached directly on `:8787` — no reverse proxy
  required for acceptable load times.

## What is different from upstream

| Area | Upstream default | This build |
|---|---|---|
| Image | prebuilt `tomicdd/artex` (backdoored) | compiled from this source tree |
| Runtime user | root | root (upstream parity) |
| Console binding | `0.0.0.0:8787` | `127.0.0.1:8787` |
| Recording proxy | published on `8788` | not published |
| Static UI delivery | uncompressed, no cache validators | Brotli/gzip + immutable cache + ETags |
| Interface language | Chinese | English |
| Browser automation | Node 22 + Playwright + Chromium | same, pinned versions |
| Browser MCP launcher | `npx @playwright/mcp` | pinned `playwright-mcp` |
| API-recon dependencies | installed during a run | preinstalled from a locked official registry |
| Online self-update | enabled, points upstream | compiled off; no remote check or download |
| Capabilities | default Docker set | default Docker set + `no-new-privileges` |
| Base images | floating tags | pinned by digest |

The API, UI, skills, browser MCP, and browser-driven recon are otherwise
unchanged. Node, Playwright, the Playwright MCP/CLI, and Chromium are
preinstalled; `nmap` raw scans and runtime `apt`/`npm -g` installs work because
the container runs as root, as upstream does. The deployment hardening that
remains is the loopback-only console, an unpublished proxy, `no-new-privileges`,
a persisted JWT key, and digest-pinned base images.

## Managed ARTEX runs

FailSafe can host and run ARTEX safely for you using **GlassBreak**, our
cyber-grade model. We handle the isolated runtime, scoping, evidence handling,
and the operational controls that are easy to get wrong in a self-hosted
deployment. We can also rent cyber-model access (including GlassBreak) for runs
on infrastructure you control.

If you want a managed run or model access, contact us:

- **Email:** [hello@getfailsafe.com](mailto:hello@getfailsafe.com)
- **Contact form:** <https://getfailsafe.com/contact-us>

## Quick start

Requirements: Docker Engine with Compose v2, and about 5 GB of free disk
(the browser stack and Chromium account for most of the image size).
If your Docker installation only provides the standalone `docker-compose`
command, substitute it for `docker compose` below; `make` detects either.

```bash
cp .env.safe.example .env.safe
chmod 600 .env.safe
# edit .env.safe: set POSTGRES_PASSWORD and, optionally, an LLM key
docker compose --env-file .env.safe -f docker-compose.safe.yml up -d --build
```

Open <http://127.0.0.1:8787> and complete the first-run setup. If you left the
LLM key blank, configure a provider in the console before starting a scan.

Check status and logs:

```bash
docker compose --env-file .env.safe -f docker-compose.safe.yml ps
docker compose --env-file .env.safe -f docker-compose.safe.yml logs -f artex
```

Stop without deleting data:

```bash
docker compose --env-file .env.safe -f docker-compose.safe.yml down
```

Delete everything, including the database and ARTEX data:

```bash
docker compose --env-file .env.safe -f docker-compose.safe.yml down -v
```

### Reaching the console from another machine

The compose file publishes the console on `127.0.0.1` only. To reach it from
your laptop, use an SSH tunnel:

```bash
ssh -L 8787:127.0.0.1:8787 user@host
```

If you put a reverse proxy in front, forward both `/` and `/api/` to the
backend; the UI's event streams use same-origin URLs in production, so no
extra port needs to be exposed.

## Build without Docker

```bash
cd web
npm ci --ignore-scripts
npm run build:static
cd ..

rm -rf server/webui/dist
mkdir -p server/webui/dist
cp -R web/out/. server/webui/dist/

CGO_ENABLED=0 go build \
  -trimpath -buildvcs=false -tags embedui \
  -ldflags="-buildid= -X main.version=v0.3.15-failsafe.3" \
  -o artex ./cmd/artex
```

Then run `./artex -addr 127.0.0.1:8787 -proxy 127.0.0.1:8788` with
`ARTEX_PG_DSN` set to a PostgreSQL database you control. This bare-binary path
does not install Node, Playwright, Chromium, or the recon CLI tools; use the
Docker image for full upstream capability.

## Verify the build

```bash
make verify
```

This checks the known implant indicators (`verifyCaptcha`, `tomicdd`,
`congdonglinux`), prints the binary SHA-256, and lists the image ID. See
[`audit/artex-v0.3.15-audit.md`](audit/artex-v0.3.15-audit.md) for the full
audit of this tag.

## Tests and continuous integration

Every push and pull request runs:

- `go vet ./...` and every Go test in its own process against a fresh
  PostgreSQL database (`ci/go-tests-isolated.py`);
- the web console's production build (`npm ci --ignore-scripts` +
  `npm run build:static`);
- an end-to-end check of the single binary with the embedded UI
  (`ci/e2e_binary.py`): static export, SPA fallback, missing-asset 404,
  Brotli/gzip, ETag `304`, immutable cache headers, authentication, task
  creation, disabled updater, and graceful shutdown;
- focused PostgreSQL regressions, the full image build, `make verify`, the
  preinstalled api-recon runtime, browser/MCP discovery, and a boot smoke test
  on an internal network.

Run the local checks yourself:

```bash
go vet ./...
ARTEX_PG_DSN=postgres://user:pass@127.0.0.1:5432/artex_test?sslmode=disable go test ./...
```

Unit tests for the static UI server (compression negotiation, ETag
revalidation, cache policy, Range/HEAD handling) run as part of `go test ./...`.

## Data and secrets

- `.env.safe` contains the database password and optional LLM keys. Keep it
  `0600` and out of version control.
- The database lives in the `pgdata` volume; ARTEX data and evidence live in
  `artex_data`; the JWT signing key lives in `artex_key`. Back up all three.
- ARTEX stores scan evidence, which may include credentials, tokens, and
  customer data. Treat `/app/data` and the database as sensitive.
- For a backup, stop the stack and archive the volumes, or use
  `pg_dump` inside the `postgres` container.

## Updating

Read [`UPSTREAM.md`](UPSTREAM.md) first. In short: review the upstream diff,
rebuild, run `make verify`, update the audit report, and only then deploy.

## License and provenance

ARTEX is AGPL-3.0. See [`LICENSE`](LICENSE) and [`UPSTREAM.md`](UPSTREAM.md).
This fork's modifications are released under the same license. The upstream
README is preserved (in English) as [`README.upstream.md`](README.upstream.md).