import { createHash } from "node:crypto";
import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";

const [, , source] = process.argv;
if (!source) throw new Error("Quellverzeichnis fehlt");

const root = path.resolve(source);
const files = [];
const excluded = new Set([".git", "node_modules", ".next", "out", "verify"]);

function collect(dir, matches) {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const file = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (!excluded.has(entry.name)) collect(file, matches);
    } else if (matches(entry.name)) {
      if (!entry.isFile()) throw new Error(`Keine reguläre Quelldatei: ${file}`);
      files.push(path.relative(root, file).split(path.sep).join("/"));
    }
  }
}

collect(root, (name) => name.endsWith(".go"));
collect(path.join(root, "web", "src"), (name) => /\.(tsx?|m?js)$/.test(name));

const digest = createHash("sha256");
for (const file of files.sort()) {
  const hash = createHash("sha256").update(readFileSync(path.join(root, file))).digest("hex");
  digest.update(file).update("\0").update(hash).update("\n");
}
console.log(digest.digest("hex"));
