# api-recon — reference manual

Grep recipes, `config.json` templates, and troubleshooting. All greps run against the `js/` directory. If a bundle is a single line, run `js-beautify` or `sed 's/}/}\n/g'` first; a raw grep with a context window is usually enough.

## Script notes

Every file in `scripts/` is a **reference template** and must be adjusted for the target site before running. Typical changes:

| Script | Common adjustments |
|---|---|
| `harvest_static.py` | endpoint regex, webpack/Vite manifest parsing, micro-frontend publicPath, retry/concurrency |
| `runtime_harvest.js` | neutralize field names and success values, stub matching rules and body shapes, route sources, WS recording, `waitUntil`/`routeTimeout`/`proxy` |
| `preload.js` | `loginPathRe`, L1 stubs, `neutralize.fields`, `apiPattern`, whether L3 is enabled, `recordDetail`, `observe.*`, `neutralizeVueRouter` |
| `spider_mpa.py` | `--exclude` destructive links, cookie, depth/max, same-origin filtering |
| `extract_route_map.py` | `routeMap` / `routeLink` regex, KEY naming patterns |
| `build_perm_tree.py` | `userRouteAuth` parsing, `ROOTS`/`PREFIX_PARENT` hierarchy heuristics, stub outer field names |
| `config.json` | the single entry point for all the site-specific parameters above |

Place adjusted files in the task working directory (e.g. `recon/`), and note the specific changes relative to the reference scripts in the report.

---

## A. Reverse the three gates

### A1. Render gate — "How is logged-in determined?"

```bash
grep -rhoaE '.{0,40}(isLogin|isAuthenticated|loggedIn|hasLogin|requireAuth)\b.{0,80}' js | head
grep -rhoaE 'function (getUser|getToken|getAuth)[0-9]?\([^)]*\)\{.{0,200}' js | head
grep -rhoaE '(localStorage|sessionStorage)\.getItem\("[^"]+"\)' js | sort -u
grep -rhoaE '(Cookies?|cookie)\.(get|load)\("[^"]+"\)' js | sort -u
grep -rhoaE '\batob\(|JSON\.parse\(|jwt|decode' js | head
```

Find the chain `isLogin = f(getUser())` → `getUser = decode(storage.read(KEY))` and determine the **storage key**, **container** (Cookie vs localStorage), and **encoding**:

| Encoding | How to forge it in config |
|---|---|
| Plain string / `"1"` / token | `"value": "anything-truthy"` |
| `JSON.parse(x)` | `"value": "json:{\"id\":1,\"username\":\"admin\"}"` |
| `JSON.parse(atob(x))` | `"value": "b64json:{\"id\":1,\"username\":\"admin\"}"` |
| JWT | Unsigned/`alg:none` JWT, or sign with a key found in the bundle |
| Encryption (SM2/AES/RSA) | Find the hardcoded key; forge if the render gate only needs a decodable blob; otherwise fall back to static |

→ Write into `cookies` / `localStorage`.

### A2. Interceptor gate — "What triggers a redirect to /login?"

```bash
grep -rhoaE '.{0,60}(interceptors\.response|axios|request\.use).{0,120}' js | head
grep -rhoaE '.{0,40}(response_code|errcode|errno|\bcode\b|\bret\b|\bstatus\b)\s*[=!]==?\s*[\-0-9]{1,4}.{0,60}' js | head -20
grep -rhoaE '.{0,40}(未登录|请重新登录|登录已过期|unauthorized|登录失效|授权|token.{0,10}invalid).{0,40}' js | head
grep -rhoaE '.{0,30}(location\.href|router\.(push|replace)|navigate)\([^)]*login[^)]*\)' js | head
```

Determine: **field name**, **success value** (usually `0` or `200`), and **the failure value that triggers a redirect**. Verify with a junk session:

```bash
curl -sk -X POST -H 'Cookie: <fakekey>=junk' https://target/api/<protected> -d '{}' -H 'Content-Type: application/json'
```

→ Write into `neutralize.fields` + `neutralize.success`.

### A3. Content gate — "Where do menus/permissions come from?"

```bash
grep -rhoaE '"/api[^"]*(permission|perm|role|menu|acl|resource|nav)[^"]*"' js | sort -u
grep -rhoaE '.{0,30}(menus|permissions|menuList|routeList|authList|role_permissions)\b.{0,120}' js | head
grep -rhoaE 'userRouteAuth|getResultTree|routeMap|routeLink|hasPermission|checkAuth' js | head
grep -rhoaE '([A-Z_][A-Z0-9_]*):\{name:"[^"]*",link:"/[^"]+"\}' js | head
```

**Two layers of data** (common in enterprise admin backends):

| API | Typical payload | Consumer |
|---|---|---|
| `.../role_permissions` | `{ permissions: string[], role_type }` | Route guards, button-level ACL |
| `.../permissions/all` | `tree[{ code, position, children }]` | Sidebar menu rendering |
| In-bundle `userRouteAuth` | `{ CODE: { url, name? } }` | code → frontend path |
| In-bundle `routeMap` | `{ KEY: { name, link } }` | Alias resolution (webpack `o.DASHBOARD`) |

Read the consumer code to confirm: how `getResultTree(tree, permissions)` filters, and which field `v-if` / `hasAuth(code)` checks.

**Manual forging** (small sites): build a permissive payload → `stubs`.

**Full permission-tree reconstruction** (large sites where the sidebar/submodules remain blank): see **section I**.

---

## B. config.json template

```json
{
  "baseUrl": "https://target/",
  "runtimeMode": "both",
  "chromium": "/usr/bin/chromium",

  "cookies": [
    { "name": "auth", "value": "b64json:{\"id\":1,\"username\":\"admin\",\"role\":\"admin\",\"func\":{},\"permissions\":[\"*\"]}" }
  ],
  "localStorage": { "token": "faketoken", "isLogin": "1" },

  "neutralize": {
    "fields": ["response_code", "code", "errno", "ret", "status"],
    "success": 0,
    "flags": { "success": true, "message": "ok" }
  },
  "forward": true,
  "loginUrlPattern": "/login",
  "apiPattern": "/api/|/rest/|/graphql",

  "mockTier": "L1+L2",
  "recordDetail": true,
  "observe": {
    "storageReads": false,
    "cookieReads": false,
    "xhrHeaders": true
  },
  "neutralizeVueRouter": true,
  "stubs": [
    {
      "match": "permissions/all|/menu|role_permissions",
      "body": {
        "response_code": 0, "code": 0,
        "data": {
          "permissions": ["*"],
          "menus": [
            { "name": "dashboard", "path": "/dashboard", "show": true, "children": [] },
            { "name": "alert", "path": "/alert", "show": true, "children": [] }
          ]
        }
      }
    }
  ],

  "explore": {
    "clickTabs": true,
    "clickTables": true,
    "pushStateFallback": true,
    "maxMenuItems": 50
  },

  "routes": ["/dashboard", "/alert", "/asset", "/device", "/report", "/config", "/system"],
  "waitMs": 1500, "perRouteMs": 900, "headless": true,
  "waitUntil": "domcontentloaded",
  "routeTimeout": 12000,
  "proxy": "",

  "captureResponses": true, "recordWs": true, "respMax": 600
}
```

Field notes:
- `runtimeMode`: `depth` (Puppeteer), `coverage` (browser MCP), `both`
- `cookies[].value` prefixes: `b64json:` → base64(JSON); `json:` → raw JSON; no prefix → literal
- `forward: true` forwards real requests and rewrites code fields; `false` is fully offline stubbing
- `mockTier`: the preload level enabled in coverage mode, e.g. `L1+L2`, `L1+L2+L3`
- `routes` comes from `routes.txt`; after forging menus the harness automatically appends `<a href>`
- `captureResponses` / `recordWs` apply only in depth mode
- `waitUntil`: use `domcontentloaded` for large SPAs to avoid hanging on `networkidle2`
- `routeTimeout`: per-route `page.goto` timeout (milliseconds)
- `proxy`: Puppeteer `--proxy-server`; `HTTP_PROXY` / `HTTPS_PROXY` can also be set

### B1. Dual-stub template (role_permissions + permissions/all)

```json
"stubs": [
  {
    "match": "role_permissions",
    "body": {
      "response_code": 0,
      "data": {
        "permissions": ["MONITOR", "MONITOR_ALERT", "THREAT", "ASSETS_RISK"],
        "role_type": "SUPER_ADMIN"
      }
    }
  },
  {
    "match": "permissions/all",
    "body": {
      "response_code": 0,
      "data": [
        {
          "code": "MONITOR",
          "position": 1,
          "children": [
            { "code": "MONITOR_ALERT", "position": 1, "children": [] }
          ]
        }
      ]
    }
  }
]
```

The outer field names (`response_code` / `code` / `data`) must match the A2 interceptor gate; `permissions` must cover every leaf code in the tree.

---

## C. coverage mode: preload configuration

Edit the `CONFIG` object at the top of `scripts/preload.js`, or replace it before CDP injection:

```javascript
const CONFIG = {
  loginPathRe: /\/(login|signin)(\/|$|\?)/i,
  mockTier: 'L1+L2',
  forward: true,
  recordDetail: true,
  extractUrlsFromResponse: true,
  neutralizeVueRouter: true,
  observe: { storageReads: false, cookieReads: false, xhrHeaders: true },
  neutralize: { fields: ['response_code', 'code'], success: 0 },
  stubs: [ /* same as config.json stubs */ ],
  apiPattern: /\/(api|apis|v\d+|dev|internal|graphql)\//i,
};
```

Verify: `window.__API_RECON_PRELOAD__ === true` and the pathname is stable.

Export the recording:

```javascript
JSON.stringify({
  apis: [...window.__API_RECON_LOG__],
  detail: window.__API_RECON_DETAIL__,
  routes: [...(window.__API_RECON_ROUTES__ || [])],
  observe: window.__API_RECON_OBSERVE__,
}, null, 2)
```

---

## D. preload / runtime hook capabilities

Browser hook capabilities built into preload (coverage) and runtime_harvest (depth), and their coverage:

| Hook capability | Value for API discovery | Coverage |
|---|---|---|
| Hook fetch / XHR.open | Record request URL/method | ✅ `recordDetail` + `__API_RECON_LOG__` |
| Hook XHR.setRequestHeader | Discover headers such as Authorization | ✅ `observe.xhrHeaders` |
| Hook localStorage/cookie reads | Confirm session key names | ⚠️ Optional `observe.storageReads/cookieReads` |
| Get Vue routes | Complete frontendRoutes | ✅ `__API_RECON_ROUTES__` (loaded routes) |
| Neutralize Vue route guards / block login redirects | Hold modules open to trigger APIs | ✅ `neutralizeVueRouter` + native navigation neutralization |
| Get React routes | Complete routes | ⚠️ Static + clicks; no dedicated hook |
| Block page navigation (login path) | Stay on the page to analyze | ⚠️ Block login paths only, to avoid blocking business navigation |
| Hook crypto libraries (CryptoJS/SM, etc.) | Encrypted parameters → plaintext API body | ❌ Hook encryption function inputs manually; record conclusions in config |
| Anti-debugging bypass | Otherwise runtime records no APIs | ❌ Handle manually; static still works |

---

## E. Endpoint extraction regex (when static results are too few)

Loosen `extract_endpoints` in `harvest_static.py`, or do it manually:

```bash
grep -rhoaE '"/[a-z][A-Za-z0-9_/\-]{3,}"' js | sort -u
grep -rhoaE '/api/[a-zA-Z0-9_./-]+' js | sort -u
```

---

## F. Troubleshooting

| Symptom | Cause → fix |
|---|---|
| Very few static APIs | Endpoint dialect mismatch → loosen the regex (section D) |
| Chunk count ≪ manifest | CSS-only or undeployed chunks; 404s already retried |
| Runtime still shows the login page | Render gate wrong → recheck A1: key name, container, encoding, domain |
| Shell entered but module blank | Content gate → forge menus (A3); `routes` paths may be wrong |
| Only bootstrap/locale per route | Permission codes incomplete → section I permission-tree reconstruction; check both `role_permissions` + `permissions/all` stubs |
| Sidebar item present but subpage blank | Tree missing intermediate nodes, or codes inconsistent with `userRouteAuth` |
| Every API redirects to login | Interceptor gate → confirm `neutralize`; nested fields need extended walk logic |
| 0 WS frames | Subscriptions may require user interaction; increase `perRouteMs` |
| Empty response body | Real responses only exist with `forward: true` |
| Chromium missing | Install chromium or set `config.chromium` / `CHROMIUM` |
| Many mocks but still redirected to login | Hook too late or missing `location.href` setter → document-start + preload |
| Lists all empty | L3 empty arrays are normal; keep clicking tabs/settings/details |
| Mistaking Redux actions for routes | Filter internal paths containing get/set/change/clear/toggle/upload |
| Vue still redirects to login | preload not at document-start → change injection timing; or clear guards manually when `neutralizeVueRouter: false` |
| URLs present in responses but not logged | Enable `extractUrlsFromResponse`; or extract manually from `__API_RECON_DETAIL__` |
| Unknown Authorization header name | Enable `observe.xhrHeaders` or inspect request headers in DevTools |
| Runtime very slow / timing out | Set `waitUntil: domcontentloaded`; lower `routeTimeout`; do not use `networkidle2` |
| Proxy connection fails | Check `proxy` / environment variables; Puppeteer and curl must use the same proxy port |

---

## G. Hardened targets

When the server validates the session progressively (unforgeable signed cookies, server-rendered menus that cannot be stubbed), runtime stalls at the shell. Expected behavior:

- **Static is enough for endpoint enumeration** — module paths are in the code
- If authorization permits, run the same harness with a **real session**: `forward: true`, no neutralize needed, capturing real methods/params/responses

---

## H. Single-task checklist

1. Confirm the authorization scope
2. **Read** `scripts/harvest_static.py` → adjust for the target → run → review `api_static.txt`, `routes.txt`
3. **Phase 1b**: path anchor windowing + binding layer → `param_candidates.json` (section J)
4. Reverse A1/A2/A3 → write the site-specific `config.json`
5. **Read and adjust** `runtime_harvest.js` / `preload.js` before running
6. `runtimeMode=depth`: `npm install` → run the adjusted harvest script
7. `runtimeMode=coverage/both`: inject the adjusted preload at document-start → browser MCP dynamic enumeration + **parameter trigger matrix**
8. Module not rendering → **section I permission-tree reconstruction** → patch stubs → rerun
9. Multi-sample parameter diff + error reverse-inference → `params_merged.json`
10. Merge → `site_map.json` + `api_merged.txt`, honestly noting coverage, gaps, and script changes

---

## I. Permission-tree reconstruction (Phase 4 in depth)

Use when forging simple `menus: [{ path, show: true }]` does not work and submodules still do not mount.

### I1. Locate the auth module

```bash
grep -l 'userRouteAuth' js/*.js
grep -l 'routeMap\|routeLink' js/*.js
grep -rhoaE 'getResultTree|role_permissions|permissions/all' js | head
```

Record: **permission API path**, **response field names**, **consuming chunk filenames**.

### I2. Extract routeMap

```bash
python3 scripts/extract_route_map.py recon/js recon/
# produces recon/route_map.json
```

If `[!] no routeMap pattern found`: loosen the regex in `extract_route_map.py`, or grep manually:

```bash
grep -rhoaE '([A-Z_][A-Z0-9_]*):\{name:"[^"]*",link:"/[^"]+"\}' js | head -20
```

### I3. Build the permission tree + stubs

```bash
python3 scripts/build_perm_tree.py recon/js recon/ --config recon/config.json
```

Script logic:
1. Parse `userRouteAuth={MONITOR:{url:...},...}` (including webpack aliases such as `He=o.DASHBOARD`)
2. Resolve aliases through `route_map.json` → real paths
3. Infer parents by code prefix (`MONITOR_ALERT` → `MONITOR`)
4. Output `permissions_tree.json`, `permissions_all_stub.json`, `role_permissions_stub.json`
5. With `--config`, automatically write into `config.json`'s `stubs` and extend `routes`

**Adjust for the target** (at the top of the script):
- `DEFAULT_ROOTS`: top-level module code list
- `DEFAULT_PREFIX_PARENT`: `PREFIX_` → parent mapping
- `DEFAULT_EXTRA_PARENT`: orphan nodes without a prefix relationship

### I4. Verify stub consistency

```bash
# The number of permissions should ≈ the number of userRouteAuth entries
wc -l recon/perm_codes_all.txt
# routes should cover every link in route_map
python3 -c "import json; m=json.load(open('recon/route_map.json')); r=set(json.load(open('recon/config.json'))['routes']); print('missing', [v['link'] for v in m.values() if v['link'] not in r])"
```

### I5. Rerun runtime and compare

```bash
node recon/runtime_harvest.js recon/config.json
# Compare runtime_api.json counts before/after forging; check whether module APIs appear for /attack, /asset, etc.
```

| Before forging | After forging (success) |
|---|---|
| The same 3–5 bootstrap entries per route | Different routes trigger different module APIs |
| Only `/api/locale/language` | Module endpoints such as `/api/web/...` appear |
| `routes.txt` has only a few routes | `routes` 80–110+ from route_map |

### I6. If still failing

- **coverage mode**: click the sidebar + tabs; permission gating may request only after interaction
- **Stub fields**: compare the nesting of a real API (curl + real session) with the stub
- **Additional guards**: grep button-level checks such as `hasPermission|checkRole|func.`, and extend `role_permissions.permissions`
- **Static fallback**: module API paths are still in `api_static.txt`; runtime only adds METHOD/body; parameters remain in `param_candidates.json` + recorded samples

---

## J. Parameter reverse engineering (Phase 1b / 5b / 5c)

**Methodology, not a universal script.** Find paths with regex; find parameters with anchor windowing + UI binding chain + multi-sample diff + error reverse-inference.

### J1. Anchor windowing — find the request-assembly object from a path

```bash
# Use a known Phase 1 path as the anchor
grep -n '"/api/user/list"' js/*.js
grep -rhoaE '.{0,120}("/api[^"]+").{0,200}' js | head
grep -rhoaE '(params|data|body|payload)\s*:\s*\{' js | head
grep -rhoaE '(get|post|put|delete|patch)\([^,]+,\s*\{' js | head
```

### J2. Wrapper layers and transport forms

```bash
# axios / unified request
grep -rhoaE '(axios|request)\.(get|post|put|delete|patch)\(' js | head
grep -rhoaE 'interceptors\.(request|response)' js | head

# GraphQL
grep -rhoaE '(query|mutation)\s+\w+|gql`|graphql\(' js | head
grep -rhoaE '\$[a-zA-Z_]+\s*:\s*(Int|String|Boolean|\[)' js | head

# FormData / multipart
grep -rhoaE 'FormData|\.append\(' js | head

# Path parameters
grep -rhoaE 'path:\s*"/[^"]*:[^"]+"' js | head
grep -rhoaE 'useParams|route\.params|\$route\.params' js | head
```

### J3. Validation gate — required / format / enum

```bash
grep -rhoaE '(required|message|pattern|enum|validator)\s*:' js | head
grep -rhoaE 'yup\.|zod\.|async-validator|Form\.Item|a-form-item|el-form-item' js | head
grep -rhoaE 'rules\s*:\s*\[|name:\s*["\'][a-zA-Z_]+["\']' js | head
grep -rhoaE 'label.*value|options\s*:\s*\[' js | head
```

### J4. Binding layer — form → API

```bash
grep -rhoaE 'onFinish|handleSubmit|getFieldsValue|validateFields' js | head
grep -rhoaE '(pick|omit|transform|dayjs|moment)\(' js | head
```

Runtime fill-in: in DevTools → Network → request → **Initiator** (call stack), follow upward from `fetch`/`send` to the request-assembly function.

### J5. Encrypted parameters

```bash
grep -rhoaE 'encrypt|decrypt|sign|CryptoJS|sm2|sm3|sm4|RSA|AES' js | head
```

**Do not guess fields from ciphertext** — hook the encryption function's **input** and record the plaintext payload before encryption; write conclusions into `config.json` / `param_candidates.json`.

### J6. Parameter trigger matrix (required in Phase 3)

Record once per operation per module, and diff request body/query:

| Operation | Focus |
|---|---|
| List first screen | Default pagination values |
| Search | keyword, filters |
| Advanced filter | optional fields |
| Create/edit | Full entity |
| Batch/export | `ids[]`, `exportType` |
| Sort/page | `sortField`, `order` |

Output `param_samples.json`: `[{ "path", "method", "action": "search", "body", "query", "headers" }]`

### J7. Confidence rules

| Confidence | Condition |
|---|---|
| **High** | Static callsite + runtime with ≥2 consistent samples |
| **Medium** | Static only, or runtime only once |
| **Low** | Response/error reverse-inference, not verified a second time |
| **Pending-trigger** | Field known statically, but the UI/permission path was not reached |

### J8. Quick scenario recipes

| Scenario | Order |
|---|---|
| REST list page | J1 request-assembly object → J6 four diffs → J3 rules |
| Create/edit form | J3 Form name → J4 submit chain → runtime submit + deliberately leave blank to see 400 |
| GraphQL | J2 variables declaration → record variables per runtime operation |
| Encrypted body | J5 hook the input → fields before encryption are the real params |

### J9. Mapping to api-recon phases

| api-recon | Parameter recon |
|---|---|
| Phase 1 static | J1 anchor windowing |
| Phase 2 A2 interceptor | Globally injected fields (tenantId, sign) |
| Phase 3 runtime | J6 trigger matrix + `param_samples.json` |
| Phase 4 permission tree | Different modules have different forms → full fields require sufficient permissions |
| Phase 5 merge | `params_merged.json` + confidence; do not determine required from a single sample |

### J10. Troubleshooting

| Symptom | Fix |
|---|---|
| Static has a field name that never appears at runtime | Mark "pending-trigger"; complete the permission tree / click advanced filters / exercise each select option |
| Same path, different body shapes | Normal — record per `action`; do not force-merge schemas |
| Stub responses are fake but you want the params | **Look at the outbound request** body/headers; do not infer from the stub response |
| 400 reports a nested field | Mind outer wrappers `data`/`bizData`/`variables` |
| GraphQL shows only operation names | Expand the `variables` JSON; find `$var: Type` statically |

---
