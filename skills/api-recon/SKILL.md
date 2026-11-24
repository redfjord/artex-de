---
name: api-recon
description: Use this skill when collecting a website's API endpoints.
---

# API Recon (frontend API reconnaissance)

Under the premise of **authorization**, discover as completely as possible: **backend APIs** (paths, methods, parameters, response bodies), **frontend routes**, and **UI feature trigger points** (tabs, dialogs, table actions, etc.).

---

## Boundaries and prohibitions (Agent must read · violations are out of scope)

This skill performs **API and parameter-surface reconnaissance only**; it is not a vulnerability-hunting or exploitation phase.

### Task boundaries

| Scope | Allowed | Prohibited |
|---|---|---|
| **Target** | Enumerate paths, methods, parameters, routes, and UI trigger points | SQLi/XSS/broken access control/brute force/fuzz vulnerabilities, packet-tampering attacks, destructive operations |
| **Authentication** | Hook + stub/mock to bypass the **client-side** login gate | Asking the user for or guessing credentials; attempting real login-form submissions |
| **Runtime** | Hook APIs without credentials and use mock responses to let the SPA enter the post-login shell | Flows that require a real backend session to continue |

### Credential-free dynamic analysis (Phase 3 default)

1. Use `preload.js` / `runtime_harvest.js` to **intercept and stub** bootstrap APIs such as login, permissions, and menus;
2. Return **structurally correct, business-code-success, data-may-be-empty** mock bodies for business query APIs;
3. Let the frontend render post-login pages even with no backend or in a 401 environment, thereby triggering more XHR/fetch/WebSocket traffic;
4. **Empty data, blank tables, and placeholder UI are all expected**—do not switch to real login or vulnerability testing for this reason.

**In one sentence**: use mocks to hold open frontend routes and component mounting, and **record only outbound requests**; what the backend returns does not matter—what matters is **which other APIs the frontend will still send**.

### Hard process prohibitions

| Prohibited | Alternative |
|---|---|
| Before Phase 1 completes, grep/curl/Read the main entry `index-*.js` to extract API paths | Run `OUTDIR/harvest_static.py` |
| Hand-writing scripts such as `extract_apis.py` to replace harvest | Modify `OUTDIR/harvest_static.py` and rerun |
| Repeating the same grep/command after it fails ≥2 times | Change strategy: read tool_logs, modify harvest, consult the reference |
| Skipping gate A/B and directly running the original `scripts/` | Copy into OUTDIR and adjust for the target |
| Real usernames/passwords, OTP, OAuth, and other authentication | stub/mock (see above) |
| Skipping stubs "to get real data" and doing access-control/injection testing | Record outbound only; that is the recon boundary |
| Delete, sensitive-data export, bulk writes, and other irreversible operations | Same for coverage clicks |
| Claiming all pages and APIs were obtained without completing runtime + dynamic enumeration | See "Definition of done" or state the limitations |
| Claiming all parameters are known without completing the parameter trigger matrix + diff | Phase 3b matrix + Phase 5 diff |
| Inferring required/optional from a single runtime sample | Multi-sample diff or validation rules/error reverse-inference |

---

## Two-layer model + run modes

| Layer | Output | Limits |
|---|---|---|
| **Static** (JS bundle) | Full endpoint paths, route drafts, candidate fields at request-assembly sites | No HTTP methods; parameters need Phase 1b; misses runtime-concatenated URLs |
| **Runtime** (live session) | Methods + bodies + responses + dynamic URLs + WS/SSE; multi-sample diff to complete parameters | Pages must actually render to send requests; a single sample cannot determine required/optional |

| Run mode | Engine | Suitable for |
|---|---|---|
| **depth** | `runtime_harvest.js` (Puppeteer) | API inventory, METHOD/params/response bodies, WS/SSE, reproducible batch runs |
| **coverage** | browser + `preload.js` | Click tabs/dialogs/tables for deeper feature-point coverage |
| **both** | depth first, then coverage | Most complete, most time-consuming |

**Parameter methodology** (no universal script): find paths with harvest/regex; find parameters with **anchor windowing + UI binding chain + multi-sample diff + error reverse-inference** (grep recipes in [reference.md](reference.md) section J).

---

## Definition of done

Recon may be claimed complete only when all of the following are satisfied:

- [ ] **Static**: Phase 1 harvest produced `api_static.txt`, `routes.txt`, `js/`
- [ ] **Runtime**: at least one of depth or coverage; coverage/both must have **working hooks + a dynamic enumeration loop**
- [ ] **In the shell**: business paths do not resolve to `/login` (mind hash routing)
- [ ] **Parameters**: coverage/both completed the parameter trigger matrix + `param_samples.json`; Phase 5 merged `params_merged.json`
- [ ] **Depth** (if module pages are blank): Phase 4 permission-tree reconstruction and rerun until **module-level APIs** appear (not just locale/bootstrap)
- [ ] **Delivery**: Phase 5 outputs are complete (see the Phase 5 output table); `insert_assets` writes service and endpoint assets

---

## Scripts and gates

`scripts/` are reference templates only; **do not** run the originals directly and treat the result as final.

**Rule**: read first → adjust for the target → write to `OUTDIR` (e.g. `recon/`) → record in `CHANGES.md`; if it does not match, rewrite following the methodology, borrowing only the structure.

| Gate | When | Reference script → OUTDIR copy | Common required changes |
|---|---|---|---|
| **A (static)** | After Phase 0, before the **first** harvest/spider run | `harvest_static.py` / `spider_mpa.py` | **The default regex works directly on most sites**; only change the endpoint regex, webpack/Vite `publicPath`, or MPA exclude/cookie when the manifest/dialect does not match |
| **B (runtime)** | After Phase 2, before running depth/coverage | `runtime_harvest.js` / `preload.js` + `config.json` | Cookie/localStorage keys, neutralize success values, stubs, login regex, API prefix, hash/history |

**Mandatory SPA order** (do not swap; Phase numbers take precedence over "explore first, script later"):

| Step | Required | Prohibited |
|---|---|---|
| After Phase 0 | The next Bash command = `python3 OUTDIR/harvest_static.py <URL> OUTDIR` | curl/grep/Read the main entry `index-*.js` (usually >500KB) |
| Gate A | Copy the script → make small changes as needed → **run immediately** | Manually extracting APIs first, then deciding whether to harvest |
| Before Phase 1 completes | Validate output with `wc -l`; on 404, modify harvest and retry | Hand-writing extract scripts; repeatedly grepping URLs that were not downloaded |
| From Phase 1b on | grep only `OUTDIR/js/*.js` | Using the main bundle instead of harvest |

- ✅ Copy `harvest_static.py` → (optionally) adjust regex → **run immediately**
- ❌ curl the main bundle → grep many times → write a temporary extract → harvest only at the end
- **MPA**: after Phase 0, the next Bash command = `python3 OUTDIR/spider_mpa.py ...`

---

## Tools and output constraints

| Constraint | Description |
|---|---|
| Large files | `index-*.js` files >100KB **must not** be Read/grepped into context; process them with OUTDIR scripts |
| grep output | Must use `\| head -20` or `-m 5`; keep only path summaries in the conversation, never paste bundle fragments |
| Validation | Use `wc -l`, `ls \| wc -l`; do not Read the whole directory |
| regex probing | Optional, ≤1 time, only on small chunks ≤50KB or HTML; official static results come from harvest |
| reference | Recipes/templates/troubleshooting are in [reference.md](reference.md); do not repeat them inline |

---

## Execution roadmap

```
Phase 0 classification + OUTDIR
  → Gate A → Phase 1 harvest (★ run immediately ★)
  → Phase 1b parameter reverse engineering
  → Phase 2 three auth gates → config.json
  → Gate B → Phase 3 runtime + parameter matrix
  → Phase 4 permission tree (if needed) → rerun Phase 3
  → Phase 5 merge report + insert_assets to insert all discovered service and endpoint API assets; discovered assets must never be omitted
```

Check items in order; **do not enter the next Phase until the previous item is complete**.

1. [ ] **Phase 0**: recon the SPA/MPA; create `OUTDIR` → [Phase 0](#phase-0--classification)
2. [ ] **Gate A + Phase 1**: copy scripts → harvest **immediately** → validate with `wc -l` → [Phase 1](#phase-1--static)
3. [ ] **Phase 1b**: anchor windowing + binding layer → `param_candidates.json` → [Phase 1b](#phase-1b--parameter-reverse-engineering)
4. [ ] **Phase 2**: three auth gates → `config.json` → [Phase 2](#phase-2--three-auth-gates)
5. [ ] **Gate B**: adjust runtime scripts → [Phase 3](#phase-3--runtime)
6. [ ] **Phase 3**: depth / coverage / both; confirm the shell is entered; parameter trigger matrix → `param_samples.json`
7. [ ] **Phase 4** (if needed): permission tree → patch stubs → rerun Phase 3 → [Phase 4](#phase-4--permission-tree-reconstruction)
8. [ ] **Phase 5**: merge outputs + report + `insert_assets` → [Phase 5](#phase-5--merge-and-report)

---

## Phase 0 — Classification

Fetch the entry HTML and **create `OUTDIR`** (do not modify the skill's `scripts/`):

- **SPA**: empty shell + `<div id=app>` + chunks → Phase 1–5
- **MPA**: SSR + `<form>`, no endpoint bundle → after gate A:

```bash
python3 recon/spider_mpa.py <BASE_URL> <OUTDIR> [--cookie "session=..."] [--max 300] [--depth 5] [--exclude "logout|delete|destroy"]
```

Produces `forms.txt`, `links.txt`, `api_inline.txt`. If an SPA has forms ≈ 0 → switch to Phase 1.

---

## Phase 1 — Static

Follow [Scripts and gates](#scripts-and-gates) · [Tools and output constraints](#tools-and-output-constraints).

```bash
python3 recon/harvest_static.py <BASE_URL> <OUTDIR>
```

harvest: parse HTML scripts → webpack/Vite manifest → download all lazy chunks → produce `js/`, `api_static.txt`, `routes.txt`, `chunkmap.txt`.

```bash
wc -l OUTDIR/api_static.txt OUTDIR/routes.txt
ls OUTDIR/js | wc -l
```

- Chunk count vs manifest: on 404, modify harvest and retry; do not manually curl each chunk
- Too few entries in `api_static.txt` → loosen the endpoint regex in OUTDIR and rerun (see reference)

### Phase 1b — Parameter reverse engineering

Paths come from Phase 1; parameter fields must be reconnoitered separately. For grep rules see [Tools and output constraints](#tools-and-output-constraints).

**Completion standard**: for important APIs, you can answer—field names, transport location, type inference, whether required, sample values, confidence.

#### 1b.0 — Transport form

| Form | Where parameters are | What to look for statically |
|---|---|---|
| REST JSON | body + query | `(params\|data\|body)\s*:\s*\{` next to the path anchor |
| GraphQL | `variables` | gql templates, `$page: Int` |
| Traditional form | urlencoded | `<form>`, `FormData` |
| File upload | multipart | `FormData.append` |
| Path parameters | `/user/:id` | route table + `useParams` / `$route.params` |
| Encrypted/signed | wrapped in `sign`/`data` | Hook the encryption function's input (reference section D) |

Output: annotate each API with `transport: query|json|form|graphql|encrypted`.

#### 1b.1 — Anchor windowing

Use a known path as an anchor, widen the window, and find the request-assembly object:

```bash
grep -n '"/api/user/list"' OUTDIR/js/*.js | head -20
grep -rhoaE '.{0,120}("/api[^"]+").{0,200}' OUTDIR/js/*.js | head -20
grep -rhoaE '(params|data|body|payload)\s*:\s*\{' OUTDIR/js/*.js | head -20
```

| Wrapper layer | Parameter clues |
|---|---|
| axios instance | `data` / `params` |
| Unified request | Interceptor injects global fields |
| OpenAPI client | Generated method signature |
| React Query / SWR | Hook's second argument |
| Vue composable | Composable input |

Type remnants: `yup`/`zod`/rules, `Form.Item name=`, embedded Swagger.

→ `param_candidates.json`: `{ path, fields[], source: "static-callsite", confidence }`

#### 1b.2 — Binding layer

```
Form field → onFinish/handleSubmit → transform → API payload
```

| Binding source | Technique |
|---|---|
| Form submit | Follow submit → transform → API |
| Table search | `getFieldsValue()` → `params` |
| Routing | `:id` / `?tab=` |
| Interceptor | Global `tenantId`, pagination, sign |
| Enum select | `options` → API enum values |

In DevTools, follow the call stack upward from `fetch`/`XHR.send` to the request-assembly function.

#### 1b.3 — Three packaging questions (≠ Phase 2 auth three gates)

| Question | What to answer |
|---|---|
| **Assembly** | Where the payload is built, traces of transforms |
| **Validation** | required, pattern, enum |
| **Transport** | path / query / body / multipart / headers |

The interceptor gate (Phase 2) also reads globally injected fields (Authorization, `X-Tenant-Id`, sign).

#### 1b.4 — Handoff to Phase 3

Candidate fields come from static analysis/the binding layer; **required/optional/conditional dependencies** must come from the Phase 3 parameter matrix + diff + Phase 5 error reverse-inference.

---

## Phase 2 — Three auth gates

grep in `OUTDIR/js/` (with `head`), and write `config.json` (recipes in the reference):

| Gate | Question | Keywords |
|---|---|---|
| **Render gate** | How is "logged in" determined? | `isLogin`, `getToken`, Cookie/localStorage |
| **Interceptor gate** | What triggers a redirect to `/login`? | `response_code`, `errno`, axios interceptor |
| **Content gate** | Where do menus/permissions come from? | `menu`, `permission`, `role`, `acl`, `routes` |

Do not treat a localStorage key name as a credential—confirm it from chunks/the request chain.

**Exit = gate B**: write conclusions into `config.json`, and modify `OUTDIR/runtime_harvest.js` / `preload.js`.

### Phase 2b — API observation (optional)

Use `preload.js` in OUTDIR to confirm session key names, Authorization, and nested API URLs:

| Config | Output |
|---|---|
| `recordDetail: true` | `__API_RECON_DETAIL__` |
| `observe.xhrHeaders: true` | Header observations |
| `extractUrlsFromResponse: true` | Nested APIs in responses |
| `observe.storageReads/cookieReads: true` | Feed back into config |
| `neutralizeVueRouter: true` | `__API_RECON_ROUTES__` |

Export each coverage round: `__API_RECON_LOG__`, `__API_RECON_DETAIL__`, `__API_RECON_ROUTES__`, `__API_RECON_OBSERVE__`.

---

## Phase 3 — Runtime

Gate B must already be passed; follow [Boundaries and prohibitions](#boundaries-and-prohibitions-agent-must-read--violations-are-out-of-scope) · credential-free mock strategy.

Set `"runtimeMode": "depth" | "coverage" | "both"` in `config.json` (templates in the reference).

### Hooks and stubs (shared by depth + coverage)

| Layer | Scope | Purpose |
|---|---|---|
| L1 precise | auth/permission/bootstrap stubs | Pass first-screen authentication |
| L2 negative correction | All JSON responses | Not-logged-in code → success |
| L3 fallback | `/api` etc. not matched by L1 | Empty success body to hold the UI open |

- **depth**: fake auth + `forward` to rewrite business codes + `stubs`; iterate `routes` (hash/history); produce `runtime_api.json`
- **coverage**: inject `preload.js` at **document-start** (CDP `addScriptToEvaluateOnNewDocument` or a Userscript)

Verify: `window.__API_RECON_PRELOAD__` exists; business paths do not return to `/login`.

```bash
cd recon && sh ensure-runtime.sh
node runtime_harvest.js config.json
```

### 3b — coverage dynamic enumeration (required)

1. Main navigation/sidebar — click each item, wait 1–3s for the network
2. Tabs — `role=tab`, `.ant-tabs-tab`
3. Tables — first-row view/edit/detail
4. Toolbars — export, filter, create (**avoid irreversible deletes**)
5. Each module entered — merge APIs/routes
6. SPA — controlled `pushState` for paths in `routes.txt` not yet covered (prohibited for MPA)

**Parameter trigger matrix** (required): record once per operation type per module, and **diff multiple samples**:

| Operation | Parameters usually added |
|---|---|
| List first screen | Pagination + default filters |
| Click search | keyword, filter |
| Advanced filter | More optional fields |
| Create/edit | Full entity |
| Batch/export/sort | `ids[]`, `exportType`, `sortField` |

**Outbound body/headers are still real under stubs**—trust the request. Record → `scan_raw.json`, `param_samples.json`, `api_detail.json`.

- **Vue**: `neutralizeVueRouter: true` + document-start preload
- **React**: `routes.txt` + sidebar clicks + `pushState`
- **both**: 3a depth first, then 3b coverage

---

## Phase 4 — Permission tree reconstruction

**Trigger**: module pages blank / only bootstrap (e.g. locale) per route → content gate not passed.

| Symptom | Meaning |
|---|---|
| Shell entered successfully | Render gate + interceptor gate passed |
| Sidebar items missing / blank on click | Stub shape or permission codes incomplete |
| Same, very few APIs per route | `v-if permission` not passing |
| `routes.txt` far shorter than the bundle | Must be completed from the auth module |

```bash
grep -rhoaE '"/api[^"]*(permission|perm|role|menu|acl)[^"]*"' OUTDIR/js/*.js | sort -u | head -30
grep -rhoaE 'userRouteAuth|getResultTree|routeMap|routeLink|menuList|authList' OUTDIR/js/*.js | head -20
```

Typical chain: `role_permissions` (flat codes) + `permissions/all` (tree) → `getResultTree` → `userRouteAuth[CODE].url`.

```bash
python3 recon/extract_route_map.py recon/js recon/
python3 recon/build_perm_tree.py recon/js recon/ --config recon/config.json
```

Intermediate outputs: `route_map.json`, `userRouteAuth.json`, `permissions_tree.json`, `*_stub.json`, `perm_codes_all.txt`.

Stub checks: the outer `response_code` matches the interceptor gate; flat codes align with the tree; `routes` covers every link in `route_map`.

After updating `config.json`, **rerun Phase 3**. For large SPAs, tune `waitUntil`, `routeTimeout`, and `perRouteMs` (see reference sections A3/I).

---

## Phase 5 — Merge and report

### Output table

| File | Phase | Content |
|---|---|---|
| `js/`, `api_static.txt`, `routes.txt`, `chunkmap.txt` | 1 | Static bundles and paths |
| `param_candidates.json` | 1b | Static parameter field candidates |
| `config.json` | 2 | Three gates + runtime configuration |
| `runtime_api.json` | 3a | depth detailed recording (including WS/SSE) |
| `param_samples.json`, `scan_raw.json`, `api_detail.json` | 3b | Multiple samples, click log, detail |
| `route_map.json`, etc. | 4 | Permission-tree intermediate files (if performed) |
| `params_merged.json` | 5 | Merged parameter fields + confidence |
| `api_merged.txt` | 5 | `METHOD /path [params] [static\|runtime\|both]` |
| `site_map.json` | 5 | Routes, APIs, params, feature points, limitations |
| **insert_assets** | 5 | Write all service and endpoint assets into the asset store |

### 5b — Parameter merge

Diff from `param_samples.json`; **there is no universal merge script**. For confidence rules see reference J7 (high/medium/low/pending-trigger).

### 5c — Error reverse-inference

Within the authorized scope, you may send incomplete requests and read the 400 response (**this is parameter recon, not vulnerability testing**): `field 'x' is required`, enum errors, etc. Note `data` wrapping, `variables`, and `bizData` before encryption.

The report must state: runtimeMode, static/runtime API counts, parameter confidence, uncovered modules, and a `CHANGES.md` summary relative to the reference scripts.

Suggested `site_map.json` structure:

```json
{
  "site": "https://example.com",
  "runtimeMode": "both",
  "appType": "vue-spa",
  "routeGuardStrategy": ["nav-neutralize", "L1-auth", "L2-patch", "forward"],
  "apisFromStatic": [],
  "apisFromRuntime": [],
  "apis": [],
  "params": [{ "method": "POST", "path": "/api/user/list", "transport": "json", "fields": [] }],
  "frontendRoutes": [],
  "routesVerifiedByClick": [],
  "featuresTriggered": [],
  "limitations": ""
}
```

For more fields and grep recipes see [reference.md](reference.md).

---

## General notes

- **Framework-agnostic**: webpack/Vite/Angular lazy-loading methods are the same
- **Transports**: REST/JSON, GraphQL, WebSocket, SSE; gRPC-web is out of scope
- **SSR**: client-side fetch can be recorded; RSC/Server Actions are not fully enumerable
- **Blind spots**: JSVMP, WASM, strong HMAC/mTLS verification → static analysis + document limitations
- **Parameter blind spots**: conditional linkage, hidden params, WASM request assembly → "pending-trigger" / "unreachable"
- **Static is the safety net**: when runtime is blocked, static can still enumerate endpoints

---

## Additional resources

- Grep recipes, `config.json` templates, troubleshooting, hooks, parameter reverse-engineering section J, site_map template: **[reference.md](reference.md)**
- Reference script paths are in the [Scripts and gates](#scripts-and-gates) table


### Runtime dependency setup

Copy `ensure-runtime.sh`, `package.json`, `package-lock.json`, and `.npmrc` with the runtime scripts. In the built image, `ensure-runtime.sh` verifies the absolute `ARTEX_PUPPETEER_PATH` and fails if it is unavailable; it does not fall back to a network install. Outside the image, the helper explicitly installs from the copied official lockfile with lifecycle scripts disabled.
