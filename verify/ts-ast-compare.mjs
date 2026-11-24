// ts-ast-compare: proves that two web/src trees differ ONLY in user-visible
// text. Every .ts/.tsx/.js/.mjs file is parsed with the project's own pinned
// TypeScript compiler (web/node_modules/typescript, see web/package-lock.json).
//
// The complete syntax tree is walked. JSX text nodes (rendered, auto-escaped
// text) are removed and the text of string literals / template-literal parts is
// replaced by a placeholder. Everything else - node kinds, identifiers,
// operators, numbers, regexes, imports, JSX tag and attribute names - must be
// identical.
//
// FAIL (security criteria): any syntax-tree difference, added/deleted file,
//   new parse error, or a URL/domain not present in the base tree.
// REVIEW (functional, listed): changed string literals in a logic position
//   (===/!==, switch case, object key, index, import, type literal,
//   fetch/require/storage call, non-display JSX attribute such as value/href).
//
// Usage (repo root, after `cd web && npm ci`):
//   node verify/ts-ast-compare.mjs <base>/web/src web/src web
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const [, , baseDir, newDir, webDir = 'web'] = process.argv;
if (!baseDir || !newDir) {
  console.log('usage: node verify/ts-ast-compare.mjs <base-src> <new-src> [web-dir]');
  process.exit(2);
}
const require = createRequire(path.resolve(webDir, 'package.json'));
const ts = require('typescript');
const K = ts.SyntaxKind;
const LIT = new Set([K.StringLiteral, K.NoSubstitutionTemplateLiteral, K.TemplateHead, K.TemplateMiddle, K.TemplateTail]);
const VALUE = new Set([K.Identifier, K.PrivateIdentifier, K.NumericLiteral, K.BigIntLiteral, K.RegularExpressionLiteral]);
const URL_RE = /\b(?:https?|wss?|ftp):\/\/[A-Za-z0-9._~:/?#[\]@!$&*+,;=%-]+/gi;
const DOM_RE = /\b[a-z0-9-]+(?:\.[a-z0-9-]+)*\.(?:com|net|org|io|cn|ru|xyz|top|app|dev|me|co|cc|tk|gg|sh)\b/gi;
const DISPLAY_ATTR = /^(aria-[a-z-]+|title|placeholder|alt|label|description|tooltip|emptyText|text|heading|subtitle|message|confirmText|cancelText)$/i;
const EQ = new Set([K.EqualsEqualsEqualsToken, K.ExclamationEqualsEqualsToken, K.EqualsEqualsToken, K.ExclamationEqualsToken]);

function logicContext(n) {
  let p = n.parent;
  if (!p) return '';
  if (ts.isBinaryExpression(p) && EQ.has(p.operatorToken.kind)) return 'comparison';
  if (ts.isCaseClause(p)) return 'switch-case';
  if (ts.isElementAccessExpression(p) && p.argumentExpression === n) return 'index';
  if ((ts.isPropertyAssignment(p) || ts.isPropertySignature(p) || ts.isPropertyDeclaration(p) || ts.isMethodDeclaration(p)) && p.name === n) return 'object-key';
  if (ts.isLiteralTypeNode(p)) return 'type';
  if (ts.isImportDeclaration(p) || ts.isExportDeclaration(p) || ts.isExternalModuleReference(p)) return 'module';
  if (ts.isCallExpression(p) && p.arguments[0] === n) {
    const callee = p.expression.getText();
    if (callee === 'import' || /(^|\.)(fetch|require|get|post|put|patch|delete|request|setItem|getItem|removeItem|open)$/.test(callee)) return `call ${callee}`;
  }
  if (ts.isJsxExpression(p)) p = p.parent;
  if (p && ts.isJsxAttribute(p)) {
    const name = p.name.getText();
    if (!DISPLAY_ATTR.test(name)) return `jsx-attr ${name}`;
  }
  return '';
}

function list(root) {
  const out = [];
  (function walk(d) {
    for (const e of fs.readdirSync(d, { withFileTypes: true })) {
      if (['node_modules', '.next', 'out'].includes(e.name)) continue;
      const p = path.join(d, e.name);
      if (e.isDirectory()) walk(p);
      else if (/\.(tsx?|m?js)$/.test(e.name)) out.push(path.relative(root, p));
    }
  })(root);
  return out.sort();
}

function analyse(file) {
  const src = fs.readFileSync(file, 'utf8');
  const kind = file.endsWith('.tsx') ? ts.ScriptKind.TSX : file.endsWith('.ts') ? ts.ScriptKind.TS : ts.ScriptKind.JS;
  const sf = ts.createSourceFile(file, src, ts.ScriptTarget.Latest, true, kind);
  const shape = [], lits = [], all = [];
  (function walk(n) {
    if (n.kind === K.JsxText) { all.push(n.getText()); return; }
    if (LIT.has(n.kind)) { shape.push(K[n.kind]); lits.push({ text: n.text, ctx: logicContext(n) }); all.push(n.text); }
    else shape.push(VALUE.has(n.kind) ? `${K[n.kind]}:${n.getText()}` : K[n.kind]);
    ts.forEachChild(n, walk);
  })(sf);
  return { shape, lits, all, errors: sf.parseDiagnostics.length };
}

function addURLs(set, s) {
  (s.match(URL_RE) || []).forEach(u => set.add(u.replace(/[.,;:!?']+$/, '').toLowerCase()));
  (s.match(DOM_RE) || []).forEach(d => set.add(d.toLowerCase()));
}

const bFiles = new Set(list(baseDir));
const nFiles = list(newDir);
let fail = false, identical = 0, textOnly = 0, changedLits = 0;
const review = [];
const bURL = new Set(), nURL = new Set();
for (const f of bFiles) if (!nFiles.includes(f)) { console.log(`FAIL deleted file: ${f}`); fail = true; }
for (const f of nFiles) {
  if (!bFiles.has(f)) { console.log(`FAIL new file: ${f}`); fail = true; continue; }
  const b = analyse(path.join(baseDir, f)), n = analyse(path.join(newDir, f));
  b.all.forEach(t => addURLs(bURL, t));
  n.all.forEach(t => addURLs(nURL, t));
  if (n.errors > b.errors) { console.log(`FAIL parse errors: ${f}`); fail = true; continue; }
  if (b.shape.length !== n.shape.length || b.shape.some((s, i) => s !== n.shape[i])) {
    console.log(`FAIL code changed: ${f}`); fail = true; continue;
  }
  let c = 0;
  b.lits.forEach((l, i) => {
    if (l.text === n.lits[i].text) return;
    c++;
    if (l.ctx) review.push(`${f} [${l.ctx}]\n      before: ${JSON.stringify(l.text).slice(0, 110)}\n      after:  ${JSON.stringify(n.lits[i].text).slice(0, 110)}`);
  });
  const jsxChanged = b.all.join('\u0000') !== n.all.join('\u0000');
  if (c === 0 && !jsxChanged) identical++; else { textOnly++; changedLits += c; }
}
const newURLs = [...nURL].filter(u => !bURL.has(u)).sort();
newURLs.forEach(u => { console.log(`FAIL new URL/domain: ${u}`); fail = true; });
if (review.length) {
  console.log(`\nREVIEW - ${review.length} changed literals in logic positions (confirm they are display text):`);
  review.forEach(r => console.log('  ' + r));
}
console.log(`\nweb files: ${nFiles.length} | syntax tree identical: ${identical} | only text changed: ${textOnly} (${changedLits} string literals + JSX text) | new URLs: ${newURLs.length}`);
if (fail) { console.log('RESULT: FAIL'); process.exit(1); }
console.log('RESULT: PASS - syntax trees, identifiers, imports and URLs are unchanged; only display text differs.');
