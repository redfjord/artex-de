#!/bin/sh
# Reproduziert den Nachweis "nur Text uebersetzt" fuer ARTEX-DE.
# Voraussetzungen: git, Go >= 1.22, Node.js >= 20, npm.
set -eu
cd "$(dirname "$0")/.."
ROOT="$(pwd)"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/artex-verification.XXXXXX")"
trap 'rm -r -- "$WORK"' EXIT
trap 'exit 1' HUP INT TERM
SOURCE="$WORK/source"
BASE="$WORK/base"
mkdir "$SOURCE"

echo "==> 1/4  Vergleichsbasis rekonstruieren und gegen den Referenzhash pruefen"
tar -cf "$WORK/source.tar" --exclude=.git --exclude=node_modules --exclude=.next \
  --exclude=out --exclude=data --exclude=keys --exclude='*.key' --exclude='*.pem' \
  --exclude=.env --exclude='.env.*' --exclude=config.json --exclude=artifacts .
tar -xf "$WORK/source.tar" -C "$SOURCE"
( cd "$SOURCE" && git apply -R --whitespace=nowarn "$ROOT/verify/ui-compatibility.patch" )
cp -R "$SOURCE" "$BASE"
( cd "$BASE" && git apply -R --whitespace=nowarn "$ROOT/verify/german-translation.patch" )
if [ "$(node verify/source-digest.mjs "$BASE")" != "$(cat verify/reference-sha256.txt)" ]; then
  echo "FAIL: Die rekonstruierte Codebasis stimmt nicht mit der festgelegten Referenz ueberein." >&2
  exit 1
fi

echo "==> 2/4  Go: Token-Vergleich (go/scanner + go/ast)"
( cd verify/go-tokens && go run . "$BASE" "$SOURCE" )

echo "==> 3/4  Web: Syntaxbaum-Vergleich mit dem projekteigenen TypeScript (web/package-lock.json)"
( cd web && npm ci --ignore-scripts --no-audit --no-fund >/dev/null )
node verify/ts-ast-compare.mjs "$BASE/web/src" "$SOURCE/web/src" web

echo "==> 4/4  Bekannte Implantat-Indikatoren im Quelltext (tomicdd-Backdoor)"
if grep -rnE 'verifyCaptcha|tomicdd|congdonglinux' --include='*.go' --include='*.ts' --include='*.tsx' agent server db cmd web/src; then
  echo "FAIL: Indikator gefunden"
  exit 1
else
  status=$?
  if [ "$status" -ne 1 ]; then
    echo "FAIL: Indikatorsuche fehlgeschlagen (grep: $status)" >&2
    exit "$status"
  fi
  echo "absent: verifyCaptcha, tomicdd, congdonglinux"
fi
echo
echo "FERTIG: Alle Pruefungen bestanden."
