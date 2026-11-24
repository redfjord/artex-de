#!/bin/sh
# In-image dependencies must already exist. Never repair an image over the network.
set -eu
if [ -n "${ARTEX_PUPPETEER_PATH:-}" ]; then
  node -e 'const p=process.env.ARTEX_PUPPETEER_PATH; if(!require("path").isAbsolute(p)) throw Error("ARTEX_PUPPETEER_PATH must be absolute"); require(p)'
else
  # Explicit local-development path: install from the copied official lockfile.
  cd "$(dirname "$0")"
  npm ci --ignore-scripts --registry=https://registry.npmjs.org/
  node -e 'require("puppeteer-core")'
fi
