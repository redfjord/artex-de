# Reviewed runtime and prompt cache validation

This change keeps the v0.3.15 application base and the existing container
capabilities. It combines the narrow security portions of PR #8 with the
prompt-cache improvement from PR #10. It does not import the v0.3.30 upgrade,
broader skill translations, new third-party model integrations, or elevated
container permissions.

## Changes and attribution

- The online-update disable, API-recon lockfile, absolute Puppeteer loader,
  image preinstallation and Puppeteer smoke test derive from PR #8 by
  `AnotherProductFromTheIdiotFactory`. The dependency preflight is corrected:
  an image with a missing preinstalled module fails without a network install.
- The worker prompt tail and deterministic deferred-tool ordering derive from
  PR #10 commit `d6bc366a285d95eb3cf70c44dc2fce6511a210f8` by Victor Nogueira
  (`victorftrdba`). They are adapted to the existing worker implementation;
  existing proxy selection, asset queries, task constraints, and tools remain.
- Global browser package installation disables lifecycle scripts and uses the
  official registry. The CLI fallback instruction uses the existing exact
  version rather than `@latest`. Global browser transitive dependencies and apt
  packages are still ecosystem-trusted; this is not a fully hermetic build.
- The pinned MCP launcher is pointed at the bundled `/usr/bin/chromium` through
  its documented executable-path environment setting. This avoids selecting a
  separate Chrome channel that is not installed in the image. Explicit user
  command-line browser settings retain their usual precedence.

## Local performance comparison

The baseline is main after PRs #5, #6, and #9:
`4e3f3104868e5d9be3510f87183b7b3dcae360a9`.
Go 1.26.10 on an Apple M1 measured five samples per case, 500 ms per sample.
Both versions assembled equivalent worker instructions, with either zero or
64 deferred tools. Figures below are sample medians.

| Case | Baseline | Changed | Baseline allocation | Changed allocation |
| --- | --- | --- | --- | --- |
| No deferred tools | 5,660 ns/op | 4,584 ns/op | 24,094 B/op | 19,083 B/op |
| 64 deferred tools | 7,554 ns/op | 6,485 ns/op | 30,609 B/op | 25,632 B/op |

Prompt assembly improved by about 19% and 14% respectively, with unchanged
allocation counts. This microbenchmark does not measure model latency,
reconnaissance completeness, scan throughput, or provider-side cache hit rates.
Custom time-dependent prompt overrides and changing constraints can still
change the prefix. No broad claim of unchanged end-to-end performance follows
from this measurement. `BenchmarkWorkerPromptAssembly` is retained for repeatable
comparison, and CI runs it on Linux.

## Required checks

- Every discovered Go test runs in its own process and fresh PostgreSQL database;
  no test is excluded by name. The local run passed 665 tests with zero failures.
  `TestLiveContextReview` skipped because no live model was configured. Database
  unavailability skips are treated as failures by the runner.
- A separate fresh-PostgreSQL job checks deterministic host-tool ordering,
  task/API lifecycle, fail-closed authentication and disabled update behavior.
- Shell verification failure injection, entrypoint lifecycle and dependency
  preflight regressions run in CI.
- The full embedded-UI Linux image is built from source and runs `make verify`.
- An internal-network smoke test boots PostgreSQL and ARTEX, checks the UI,
  authenticated API, rejected unauthenticated access, disabled updater,
  Puppeteer, MCP discovery and actual browser navigation to a local fixture,
  raw SYN scanning of the container's own listening port, graceful SIGTERM and
  JWT validity after restart. It publishes no ports and supplies no LLM key.

The prior shared-database suite exposed an obsolete assertion against
`coverage.scope`, a missing metering-table fixture setup and background fixture
contamination. The scope regression now verifies every configured rule through
the existing `list_companies` agent tool and retains the asset/backlog/worker
checks. The metering test performs the same table setup as normal application
startup. Fresh process/database isolation prevents one test's saved tasks and
provider settings from being reloaded by another. These are test corrections;
they do not remove scope data or change production database initialization.

## Security limits

Source inspection and known-implant checks are useful evidence but cannot prove
that a source tree, compiler, dependency, browser, image, or agent has no backdoor.
This change adds no new model service or agent endpoint, and keeps the console
loopback-only, proxy unpublished, default seccomp and `no-new-privileges`.
ARTEX retains intentional shell, root, browser and network execution capabilities.
Use a disposable execution boundary and enforce engagement network scope outside
the agent-controlled process. Release signing, SBOM/provenance verification and
broader isolation remain separate work.
