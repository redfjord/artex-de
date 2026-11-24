package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/db"
	actool "github.com/Autumn-27/norma/tool"
)

// assetInterceptCandidates 提取一条待插入资产输入项的 域名/IP/URL 候选串，用于资产拦截匹配。
// URL 的 host 会拆出归类，使「只带 URL」的服务/端点资产也能被 域名/IP 规则命中。
func assetInterceptCandidates(item assetInputItem) (domains, ips, urls []string) {
	add := func(dst *[]string, s string) {
		if s = strings.TrimSpace(s); s != "" {
			*dst = append(*dst, s)
		}
	}
	add(&domains, item.Domain)
	for _, d := range item.BoundDomains {
		add(&domains, d)
	}
	add(&ips, item.IP)
	add(&ips, item.ServiceIP)
	add(&urls, item.URL)
	if item.URL != "" {
		if u, err := url.Parse(item.URL); err == nil {
			if h := u.Hostname(); h != "" {
				if net.ParseIP(h) != nil {
					add(&ips, h)
				} else {
					add(&domains, h)
				}
			}
		}
	}
	return domains, ips, urls
}

// assetInputLabel 返回一条待插入资产的简短标识，用于拦截说明消息。
func assetInputLabel(item assetInputItem) string {
	typ := strings.TrimSpace(item.Type)
	var target string
	switch {
	case strings.TrimSpace(item.Domain) != "":
		target = strings.TrimSpace(item.Domain)
	case strings.TrimSpace(item.URL) != "":
		target = strings.TrimSpace(item.URL)
	case strings.TrimSpace(item.IP) != "":
		target = strings.TrimSpace(item.IP)
	case strings.TrimSpace(item.ServiceIP) != "":
		target = strings.TrimSpace(item.ServiceIP)
	default:
		target = "(unbekannt)"
	}
	if typ != "" {
		return fmt.Sprintf("[%s] %s", typ, target)
	}
	return target
}

// =====================================================================
// Unified asset insertion tools
// =====================================================================

// SetAssetStore wires the asset store and company store onto this ToolSet
// so the insert_assets, add_company_scope, and list_assets tools are active.
func (t *ToolSet) SetAssetStore(as *db.AssetStore, cs *db.CompanyStore) {
	t.as = as
	t.cs = cs
}

// assetInputItem is one element of the insert_assets "assets" array.
type assetInputItem struct {
	Type string `json:"type"` // root_domain|ip|subdomain|app|service|endpoint

	// ---- root_domain / subdomain ----
	Domain      string   `json:"domain"`
	ICP         string   `json:"icp"`
	RecordType  string   `json:"record_type"`
	RecordValue []string `json:"record_value"`

	// ---- ip ----
	IP           string           `json:"ip"`
	BoundDomains []string         `json:"bound_domains"`
	OpenPorts    []db.PortService `json:"open_ports"`

	// ---- app ----
	AppName     string `json:"app_name"`
	BundleID    string `json:"bundle_id"`
	Category    string `json:"category"`
	Description string `json:"description"`
	AppICP      string `json:"app_icp"`
	CompanyID   *int64 `json:"company_id"` // explicit company link (app only; others auto-attribute via scope)

	// ---- service (http) ----
	URL           string           `json:"url"`
	Technologies  []string         `json:"technologies"`
	StatusCode    *int             `json:"status_code"`
	ContentLength *int64           `json:"content_length"`
	PageTitle     string           `json:"page_title"`
	FaviconMMH3   string           `json:"favicon_mmh3"`
	Auth          []map[string]any `json:"auth"`
	ServiceName   string           `json:"service_name"`
	ServiceIP     string           `json:"service_ip"` // optional enrichment IP

	// ---- service (other) ----
	Port  int    `json:"port"`
	Proto string `json:"proto"`

	// ---- endpoint ----
	Method string           `json:"method"`
	Params []map[string]any `json:"params"`
}

// insertAssets is the unified insert_assets agent tool.
func (t *ToolSet) insertAssets() actool.CoreTool {
	return writeTool(
		"insert_assets",
		"Massenregistrierung neu entdeckter Assets, mehrere Typen mischbar (type siehe Enum).\n"+
			"Pflichtfelder je Typ: root_domain→domain; ip→ip (muss IPv4/IPv6 sein, kein Hostname); subdomain→domain; app→app_name; service(HTTP)→url; service(nicht-HTTP)→service_name+port (ip/domain mind. eines); endpoint→url+method. Übrige Felder siehe jeweilige Erklärung.\n"+
			"auth/technologies/params werden angehängt (append), nicht überschrieben.\n"+
			"Rückgabe: {results:[{index,id,type}], errors:[{index,error}]}",
		obj(map[string]any{
			// task_id 不暴露给模型：worker 归属哪个 task 由程序经 SetTaskID 权威赋值(见 handler)。
			"assets": map[string]any{
				"type":        "array",
				"description": "Asset-Array, jedes Element entspricht einem Asset-Datensatz",
				"items": obj(map[string]any{
					"type": map[string]any{
						"type":        "string",
						"enum":        []string{"root_domain", "ip", "subdomain", "app", "service", "endpoint"},
						"description": "Asset-Typ",
					},
					// root_domain / subdomain
					"domain":      str("Root-Domain oder Subdomain (root_domain/subdomain erforderlich)"),
					"icp":         str("ICP-Beian-Nummer (optional)"),
					"record_type": str("DNS-Auflösungstyp: A/AAAA/CNAME/MX etc. (subdomain optional)"),
					"record_value": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Liste der DNS-Auflösungswerte (subdomain optional, z.B. [\"1.2.3.4\",\"2.3.4.5\"])",
					},
					// ip
					"ip": str("IP-Adresse, muss IPv4/IPv6 sein, kein Hostname (Hostname bitte im domain-Feld mit type=subdomain verwenden); ip-Typ Pflichtfeld; service/endpoint-Typ optional, zur Verknüpfung mit IP"),
					"bound_domains": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Liste der an diese IP gebundenen Domains (ip-Typ optional)",
					},
					"open_ports": map[string]any{
						"type":        "array",
						"description": "Liste offener Ports (ip-Typ optional)",
						"items": obj(map[string]any{
							"port":    intp("Portnummer"),
							"service": str("Dienstname, z.B. http/ssh/mysql (optional)"),
						}, "port"),
					},
					// app
					"app_name":    str("Anwendungsname (app-Typ Pflichtfeld)"),
					"bundle_id":   str("Bundle ID (app-Typ optional)"),
					"category":    str("Anwendungskategorie (optional)"),
					"description": str("Anwendungsbeschreibung (optional)"),
					"app_icp":     str("App ICP-Beian (optional)"),
					"company_id":  intp("Zugehörige Unternehmens-ID (app-Typ optional; App kann nicht automatisch über Scope zugeordnet werden, muss explizit angegeben werden. ID wird von add_company_scope zurückgegeben)"),
					// service (http)
					"url":         str("Vollständige URL inkl. Protokoll und Port (HTTP-Service Pflichtfeld; service_type wird automatisch auf http gesetzt)"),
					"status_code": intp("HTTP-Antwortstatuscode, z.B. 200/301/403/404 (optional)"),
					"content_length": map[string]any{
						"type":        "integer",
						"description": "HTTP-Antwortkörper in Bytes (optional)",
					},
					"page_title":   str("Seiten-<title>-Inhalt (optional)"),
					"favicon_mmh3": str("favicon MMH3-Hash (optional)"),
					"technologies": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Fingerprint/Tech-Stack-Liste, z.B. [\"Nginx\",\"Vue\",\"Bootstrap\"] (optional)",
					},
					"auth": map[string]any{
						"type":        "array",
						"description": "Liste gefundener Authentifizierungsinformationen, jede mit Feldern wie type/username/password (optional, wird angehängt, nicht überschrieben)",
						"items":       map[string]any{"type": "object"},
					},
					// service (other，非 HTTP)
					"service_name": str("Dienstname, z.B. ssh/mysql/redis (Pflichtfeld für service, nicht-HTTP)"),
					"port":         intp("Portnummer (Pflichtfeld für service, nicht-HTTP)"),
					// endpoint
					"method": str("HTTP-Methode: GET/POST/PUT/PATCH/DELETE etc. (endpoint Pflichtfeld)"),
					"params": map[string]any{
						"type":        "array",
						"description": "Liste der Anfrageparameter, jedes mit location(query/body/header/path)/name/value/type (optional, wird angehängt, nicht überschrieben)",
						"items":       map[string]any{"type": "object"},
					},
				}, "type"),
			},
		}, "assets"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if t.as == nil {
				return actool.Errorf("insert_assets nicht aktiv: AssetStore nicht initialisiert"), nil
			}
			var a struct {
				Assets []assetInputItem `json:"assets"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			// task_id 由程序权威赋值(worker: SetTaskID)，不接受模型传入——避免模型漏传/错传
			// 导致资产未归任务或归错任务。无任务上下文的调用方(auto/pentest/chat)其 t.taskID=0。
			taskID := t.taskID

			type result struct {
				Index int    `json:"index"`
				ID    int64  `json:"id"`
				Type  string `json:"type"`
			}
			type errEntry struct {
				Index int    `json:"index"`
				Error string `json:"error"`
			}

			var results []result
			var errs []errEntry

			// 资产闸门规则一次性载入；读取失败则跳过判定（不阻断插入）。
			// 拦截规则 = 全局 ∪ 任务级 block；允许规则 = 任务级 allow。
			blockRules, _ := t.as.ListAssetInterceptRules()
			var allowRules []db.AssetInterceptRule
			if t.taskID > 0 {
				if tb, ta, err := t.as.TaskInterceptRulesSplit(t.taskID); err == nil {
					blockRules = append(blockRules, tb...)
					allowRules = ta
				}
			}

			for i, item := range a.Assets {
				// 资产闸门：先拦截后允许，被拒的资产禁止插入（跳过 Upsert 及后续副作用）。
				domains, ips, urls := assetInterceptCandidates(item)
				if d := db.EvaluateAssetGate(blockRules, allowRules, domains, ips, urls); !d.Allowed {
					errs = append(errs, errEntry{
						Index: i,
						Error: fmt.Sprintf("Asset %s %s, Einfügung verweigert", assetInputLabel(item), d.Reason),
					})
					continue
				}

				typ := strings.TrimSpace(item.Type)
				var id int64
				var err error

				switch typ {
				case "root_domain":
					id, err = t.as.UpsertRootDomain(db.UpsertRootDomainReq{
						Domain: item.Domain,
						ICP:    item.ICP,
						TaskID: taskID,
					})

				case "ip":
					id, err = t.as.UpsertIP(db.UpsertIPReq{
						IP:           item.IP,
						BoundDomains: item.BoundDomains,
						OpenPorts:    item.OpenPorts,
						TaskID:       taskID,
					})

				case "subdomain":
					id, err = t.as.UpsertSubdomain(db.UpsertSubdomainReq{
						Domain:      item.Domain,
						RecordType:  item.RecordType,
						RecordValue: item.RecordValue,
						ICP:         item.ICP,
						TaskID:      taskID,
					})

				case "app":
					id, err = t.as.UpsertApp(db.UpsertAppReq{
						Name:        item.AppName,
						BundleID:    item.BundleID,
						Category:    item.Category,
						Description: item.Description,
						ICP:         item.AppICP,
						CompanyID:   item.CompanyID,
						TaskID:      taskID,
					})

				case "service":
					// distinguish HTTP vs other by presence of url
					if item.URL != "" {
						// agent may send "ip" or "service_ip" for the enrichment IP; accept both
						svcIP := item.ServiceIP
						if svcIP == "" {
							svcIP = item.IP
						}
						id, err = t.as.UpsertHTTPService(db.UpsertHTTPServiceReq{
							URL:           item.URL,
							Technologies:  item.Technologies,
							StatusCode:    item.StatusCode,
							ContentLength: item.ContentLength,
							PageTitle:     item.PageTitle,
							FaviconMMH3:   item.FaviconMMH3,
							Auth:          item.Auth,
							IP:            svcIP,
							TaskID:        taskID,
						})
					} else {
						id, err = t.as.UpsertOtherService(db.UpsertOtherServiceReq{
							Domain:      item.Domain,
							IP:          item.IP,
							Port:        item.Port,
							ServiceName: item.ServiceName,
							Auth:        item.Auth,
							TaskID:      taskID,
						})
					}

				case "endpoint":
					id, err = t.as.UpsertEndpoint(db.UpsertEndpointReq{
						URL:    item.URL,
						Method: item.Method,
						Params: item.Params,
						IP:     item.ServiceIP,
						TaskID: taskID,
					})

				default:
					errs = append(errs, errEntry{Index: i, Error: "unknown type: " + typ})
					continue
				}

				if err != nil {
					errs = append(errs, errEntry{Index: i, Error: err.Error()})
					continue
				}
				results = append(results, result{Index: i, ID: id, Type: typ})
				t.writes.Assets++
				t.anchorOwner(id)
				if taskID > 0 {
					var sourceNodeID *int64
					if t.ownerNode > 0 {
						nodeID := t.ownerNode
						sourceNodeID = &nodeID
					}
					summary := "Agent registriert via insert_assets"
					if t.ownerNode > 0 {
						summary = fmt.Sprintf("Worker Intent #%d registriert via insert_assets", t.ownerNode)
					}
					_ = t.as.SetTaskAssetSource(taskID, id, "agent", summary, sourceNodeID)
				}
				// 自动入测试范围(source='auto')：只对 worker 顶层显式插入的这一项，按其
				// 类型加保守范围；side-effect 派生的资产不经此处，故范围不盲目扩大。taskID=0 时无操作。
				// 与覆盖度开关无关：task_scope 是任务的范围边界(list/查询的过滤基准)，
				// 覆盖度开关只决定要不要把它当分母去算指标，不决定要不要累积范围本身。
				{
					svcIP := item.ServiceIP
					if svcIP == "" {
						svcIP = item.IP
					}
					_ = t.as.AddAutoScope(taskID, typ, item.Domain, item.URL, svcIP)
				}
			}

			return jsonResult(map[string]any{
				"results": results,
				"errors":  errs,
			})
		},
	)
}

// addCompanyScope writes to company_scope table and triggers asset attribution.
func (t *ToolSet) addCompanyScope() actool.CoreTool {
	return writeTool(
		"add_company_scope",
		"Domain/IP/CIDR/ICP-Beian/Unternehmensschlagwörter zum [Asset-Bereich] einer Firma hinzufügen — Domain, Netz und ICP übernehmen getroffene Assets automatisch, Schlagwörter dienen nur als Bereichshinweis für den Agenten.\n"+
			"Firmenname eindeutig: company nicht existent → neu, existent → wiederverwenden (nur Bereich zusammenführen).\n"+
			"scope eine Zeile pro Eintrag, System erkennt automatisch: Root-Domain / URL / einzelne IP / CIDR-Netz / ICP-Beian / Unternehmensschlagwort.\n"+
			"reason mit Zugehörigkeitsbegründung zwingend angeben (whois/Zertifikat/ASN etc.).\n"+
			"Schutz: nackte TLD und zu breite Netze abgelehnt (IPv4-Präfix /16-/32, IPv6-Präfix /32-/128), ungültige Zeilen werden übersprungen und in errors zurückgegeben.",
		obj(map[string]any{
			"company": str("Firmenname (wird neu erstellt wenn nicht existent, sonst wiederverwendet; Name eindeutig)"),
			"scope":   str("Asset-Bereich, eine Zeile pro Eintrag: Domain / URL / IP / CIDR / ICP-Beian / Unternehmensschlüsselwort"),
			"reason":  str("Zugehörigkeitsgrundlage (Beweis/Quelle), unbedingt ausfüllen"),
			"logo":    str("Firmenlogo-URL (optional; nur bei Neuerstellung wirksam)"),
		}, "company", "scope"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if t.cs == nil {
				return actool.Errorf("add_company_scope nicht aktiv: CompanyStore nicht initialisiert"), nil
			}
			var a struct {
				Company string `json:"company"`
				Scope   string `json:"scope"`
				Reason  string `json:"reason"`
				Logo    string `json:"logo"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if strings.TrimSpace(a.Company) == "" {
				return actool.Errorf("company darf nicht leer sein"), nil
			}
			companyID, _, err := t.cs.UpsertCompany(a.Company, a.Logo)
			if err != nil {
				return actool.Errorf("Firma erstellen/abrufen fehlgeschlagen: " + err.Error()), nil
			}
			lines := splitLines(a.Scope)
			added, skipped, invalid, errMsgs := t.cs.AddScope(companyID, lines, a.Reason)
			out := map[string]any{
				"company_id": companyID,
				"added":      added,
				"skipped":    skipped,
				"invalid":    invalid,
			}
			if len(errMsgs) > 0 {
				out["errors"] = errMsgs
			}
			return jsonResult(out)
		},
	)
}

// addTaskScope lets the plan agent add test scope to THE CURRENT TASK — the coverage
// denominator and the task's authorization edge. Worker discoveries are auto-scoped
// (precise host) by insertAssets; this tool is for DELIBERATELY WIDENING: pull a whole
// root domain or whole company into scope, or add a specific subdomain / ip.
func (t *ToolSet) addTaskScope() actool.CoreTool {
	return writeTool(
		"add_task_scope",
		"Testbereich zu [dieser Aufgabe] hinzufügen — dies ist die Autorisierungsgrenze der Aufgabe und der Nenner der Asset-Testabdeckung.\n"+
			"kind unterstützt: company (alle Assets der Firma) / root_domain (gesamte Root-Domain, inkl. aller Subdomains) / subdomain (einzelne exakte Subdomain) / ip / cidr / icp / keyword.\n"+
			"Hinweis: Hosts, die der Worker einzeln trifft, werden vom System [automatisch] in den Bereich aufgenommen (exakte Subdomain); dieses Werkzeug dient der [aktiven Erweiterung] — gesamte Root-Domain/ganze Firma aufnehmen oder bestimmte Subdomain/IP ergänzen.\n"+
			"value: company → Firmenname oder ID (Firma muss existieren); root_domain/subdomain → Domain; ip/cidr → IP oder Netz; icp/keyword → Beian-Nummer oder Unternehmensschlagwort.\n"+
			"reason mit Begründung zwingend angeben (auditierbar). Mehrere Einträge via entries-Array.",
		obj(map[string]any{
			"entries": map[string]any{"type": "array", "description": "Batch: [{kind, value}]. kind∈company/root_domain/subdomain/ip/cidr/icp/keyword.", "items": map[string]any{"type": "object"}},
			"kind":    str("[Einzeln] company / root_domain / subdomain / ip / cidr / icp / keyword"),
			"value":   str("[Einzeln] Firmenname oder ID / Domain / IP / CIDR / ICP / Schlüsselwort"),
			"reason":  str("Aufnahmegrund (für Audit), unbedingt ausfüllen"),
		}),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if t.as == nil {
				return actool.Errorf("add_task_scope nicht aktiv: AssetStore nicht initialisiert"), nil
			}
			if t.taskID <= 0 {
				return actool.Errorf("add_task_scope benötigt Aufgabenkontext (aktuell kein task)"), nil
			}
			type scopeEntry struct {
				Kind  string `json:"kind"`
				Value string `json:"value"`
			}
			var a struct {
				Entries    []scopeEntry `json:"entries"`
				scopeEntry              // 单条模式
				Reason     string       `json:"reason"`
			}
			_ = json.Unmarshal(in, &a)
			items := a.Entries
			if len(items) == 0 {
				items = []scopeEntry{a.scopeEntry}
			}
			var added []map[string]any
			errs := map[string]string{}
			for i, e := range items {
				ts, err := t.as.AddAgentScope(t.taskID, strings.TrimSpace(e.Kind), e.Value, a.Reason, "agent")
				if err != nil {
					errs[strconv.Itoa(i)] = err.Error()
					continue
				}
				added = append(added, map[string]any{"kind": ts.Kind, "domain": ts.Domain, "net": ts.Net, "value": ts.Value, "company_id": ts.CompanyID})
			}
			out := map[string]any{"added": added}
			if len(errs) > 0 {
				out["errors"] = errs
			}
			return jsonResult(out)
		},
	)
}

// listUntestedAssets lets the plan agent pull the current + directly inherited
// scope's not-yet-tested assets on demand (filter by type, paginated).
func (t *ToolSet) listUntestedAssets() actool.CoreTool {
	return readTool(
		"list_untested_assets",
		"Abfrage von Assets im Bereich [dieser Aufgabe und direkt verbundener Aufgaben], die noch nicht von Faktenankern abgedeckt sind (verbundener Bereich schreibgeschützt, für eigene Entscheidung ob Nachtest nötig, ersetzt nicht deine Entscheidung).\n"+
			"Optional nach Asset-Typ filtern: root_domain/subdomain/service/app/endpoint/ip.\n"+
			"Paginierung: page ab 1, page_size Standard 10. Rückgabe {assets:[{id,type,label}], total, page, page_size}. Nur im Aufgabenkontext nutzbar.",
		obj(map[string]any{
			"type":      str("Asset-Typ-Filter (optional): root_domain/subdomain/service/app/endpoint/ip"),
			"page":      intp("Seitenzahl, ab 1 (Standard 1)"),
			"page_size": intp("Anzahl pro Seite (Standard 10)"),
		}),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if t.as == nil {
				return actool.Errorf("list_untested_assets nicht aktiv: AssetStore nicht initialisiert"), nil
			}
			if t.taskID <= 0 || t.ts == nil {
				return actool.Errorf("list_untested_assets benötigt Aufgabenkontext"), nil
			}
			var a struct {
				Type     string `json:"type"`
				Page     int    `json:"page"`
				PageSize int    `json:"page_size"`
			}
			_ = json.Unmarshal(in, &a)
			if a.Page <= 0 {
				a.Page = 1
			}
			if a.PageSize <= 0 {
				a.PageSize = 10
			}
			offset := (a.Page - 1) * a.PageSize
			assets, total, err := t.as.ListUntestedAssetsWithSources(t.taskID, strings.TrimSpace(a.Type), a.PageSize, offset)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return jsonResult(map[string]any{
				"assets": assets, "total": total, "page": a.Page, "page_size": a.PageSize,
			})
		},
	)
}

// listAssets lets an agent query the asset table.
func (t *ToolSet) listAssets() actool.CoreTool {
	return readTool(
		"list_assets",
		"Asset-DB abfragen: DSL-Ausdruckssuche oder direkt per id/ids; Paginierung unterstützt. Nur Assets im Testbereich [dieser Aufgabe und direkt verbundener Aufgaben] werden zurückgegeben.\n"+
			"DSL: field=value Fuzzy (ILIKE) | field==value exakt | field!=value Ausschluss | Zahlenfelder unterstützen > >= < <= | Bloßes Wort=Volltext-Fuzzy; AND/OR Kombination (AND Priorität höher), Klammern gruppieren. Asset-Typ über separaten type-Parameter, nicht in DSL.\n"+
			"Ohne id/ids muss dsl nicht leer sein (bedingungslose Vollabfrage nicht erlaubt).\n"+
			"Verfügbare Felder: domain(Root/Sub/Service-Domain), root_domain, ip, url, page_title, icp, service_name, app_name, method (z. B. GET/POST), service_type(http|other), record_type (z. B. A/CNAME), technology (Array, =fuzzy ==exakt), port/status_code/company_id (Integer).\n"+
			"Beispiel: status_code>=400 AND technology=shiro ; (port==80 OR port==443) AND technology=nginx",
		obj(map[string]any{
			"dsl":    str(`DSL-Abfrageausdruck (Syntax/Felder siehe Tool-Beschreibung). Muss nicht leer sein, wenn id/ids nicht übergeben werden.`),
			"type":   str("Asset-Typ-Filter: root_domain|ip|subdomain|app|service|endpoint (eigenes Feld, kann mit dsl kombiniert; allein type reicht nicht für Abfrage, dsl weiterhin erforderlich)"),
			"id":     intp("Direkt nach einzelner Asset-ID abrufen (optional, gegenseitig ausschließend mit dsl/type)"),
			"ids":    map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Direkt nach mehreren Asset-IDs abrufen (optional, gegenseitig ausschließend mit dsl/type)"},
			"limit":  intp("Rückgabelimit, Standard 10 (optional)"),
			"offset": intp("Paginierungsversatz, Standard 0 (optional)"),
		}),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if t.as == nil {
				return actool.Errorf("list_assets nicht aktiv: AssetStore nicht initialisiert"), nil
			}
			var a struct {
				DSL    string  `json:"dsl"`
				Type   string  `json:"type"`
				ID     int64   `json:"id"`
				IDs    []int64 `json:"ids"`
				Limit  int     `json:"limit"`
				Offset int     `json:"offset"`
			}
			_ = json.Unmarshal(in, &a)
			if a.Limit <= 0 {
				a.Limit = 10
			}

			var assets []*db.Asset
			var err error
			switch {
			case a.ID > 0:
				assets, err = t.as.GetByIDsInScope(t.taskID, []int64{a.ID})
			case len(a.IDs) > 0:
				assets, err = t.as.GetByIDsInScope(t.taskID, a.IDs)
			case a.DSL != "":
				assets, err = t.as.QueryDSLInScope(a.DSL, a.Type, t.taskID, a.Limit, a.Offset)
			default:
				return actool.Errorf("Ohne id/ids darf dsl nicht leer sein: bedingungslose Abfrage aller Assets nicht erlaubt, bitte Abfragebedingung angeben"), nil
			}
			if err != nil {
				return actool.Errorf("DSL-Fehler: " + err.Error()), nil
			}
			return jsonResult(map[string]any{
				"count":  len(assets),
				"assets": assets,
			})
		},
	)
}

// listCompanies lets an agent enumerate companies (企业) with their scope + asset count.
func (t *ToolSet) listCompanies() actool.CoreTool {
	return readTool(
		"list_companies",
		"Listet die 【Unternehmen/Firmen】 im Asset-Bestand mit ihrem Asset-Bereich (scope) und der Anzahl zugeordneter Assets auf. Dient dazu, welche Firmen existieren einzusehen und "+
			"die company_id zu erhalten (für insert_assets zur app-Verknüpfung, für list_assets zur Filterung nach company_id)."+
			"Optionales search filtert per Firmenname fuzzy (case-insensitiv), leer lässt alle zurückgeben.",
		obj(map[string]any{
			"search": str("Fuzzy-Filter nach Firmenname (optional, case-insensitive); leer lassen für alle"),
		}),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if t.cs == nil {
				return actool.Errorf("list_companies nicht aktiviert: CompanyStore nicht initialisiert"), nil
			}
			var a struct {
				Search string `json:"search"`
			}
			_ = json.Unmarshal(in, &a)
			cos, err := t.cs.ListCompanies()
			if err != nil {
				return actool.Errorf("Abfrage der Unternehmen fehlgeschlagen: " + err.Error()), nil
			}
			q := strings.ToLower(strings.TrimSpace(a.Search))
			type companyOut struct {
				ID         int64    `json:"id"`
				Name       string   `json:"name"`
				AssetCount int      `json:"asset_count"`
				Scope      []string `json:"scope"`
			}
			out := make([]companyOut, 0, len(cos))
			for _, c := range cos {
				if q != "" && !strings.Contains(strings.ToLower(c.Name), q) {
					continue
				}
				scope := make([]string, 0, len(c.Scope))
				for _, r := range c.Scope {
					scope = append(scope, r.Raw)
				}
				out = append(out, companyOut{ID: c.ID, Name: c.Name, AssetCount: c.AssetCount, Scope: scope})
			}
			return jsonResult(map[string]any{"count": len(out), "companies": out})
		},
	)
}

// splitLines splits a multi-line string into non-empty trimmed lines.
func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// WorkerTools returns the tool set for a work agent.
func (t *ToolSet) WorkerTools() []actool.CoreTool {
	return []actool.CoreTool{
		// list_findings 保留：报漏洞前先查本任务已确认漏洞，避免重复上报同一漏洞。
		t.listFindings(),
		t.addFinding(), t.recordFact(),
		// asset management (handlers guard nil store internally)。
		// add_company_scope 不给 worker：定义企业资产范围属规划/主控/Auto 的职责，worker 只执行探索。
		t.insertAssets(), t.listAssets(),
		// 跨 work 回看：worker 也可复用其他 work 的观察，避免重复劳动。
		// search_all_worker_traces：不必先知道 intent_id，按关键字全局捞命中步骤；
		// get_worker_trace：锁定某条 work 后列步骤/就地搜/取完整内容。
		t.searchAllWorkerTraces(), t.getWorkerTrace(),
		// node_detail：worker 拿到 intent_id/节点 id 后可查该节点完整详情（配合上面的回看）。
		t.nodeDetail(),
		// 以下工具仍【不给】worker，只留给 planner/main（读上下文、跨 work 复盘是规划职责，
		// worker 只做单条意图的执行与写回）：list_facts / list_companies / list_worker_traces。
	}
}

// MainAgentTools returns the human-interface tool set.
func (t *ToolSet) MainAgentTools() []actool.CoreTool {
	return []actool.CoreTool{
		t.graphOverview(), t.listFindings(), t.listFacts(), t.nodeDetail(),
		t.expandDigest(), // cold-digest §6.1
		t.getWorkerOutput(), t.getWorkerTrace(), t.searchAllWorkerTraces(), t.addHint(), t.addIntent(),
		// steer_work：人可对某条正在运行的意图(work)实时注入纠偏指令（不打断、不丢进展）。
		t.steerWorkTool(),
		// set_goals：人可在运行时给本任务补一个新的最终目标（规划者据此重判是否达成）。
		t.setGoals(),
		// set_constraints：人可在运行时给本任务补/改操作约束（allow/deny），约束 planner/worker 的探索边界。
		t.setConstraints(),
		// asset management (handlers guard nil store internally)
		t.insertAssets(), t.addCompanyScope(), t.listAssets(),
		t.addFinding(), t.recordFact(),
		t.addTaskScope(),
		// list_untested_assets：按需查本任务范围内未测资产(类型+分页)，自行决定补测。
		t.listUntestedAssets(),
	}
}

// AllDomainTools returns the union of all domain tools across all agent types,
// deduped by name (mainagent order wins). Used by the server to build a registry
// for injecting domain tools into agents (Auto, custom) that don't own a per-task
// ToolSet. The caller provides real stores; tools are callable at taskID=0 scope.
func (t *ToolSet) AllDomainTools() []actool.CoreTool {
	seen := map[string]bool{}
	var out []actool.CoreTool
	all := append(append(t.MainAgentTools(), t.PlannerTools()...), t.WorkerTools()...)
	for _, tool := range all {
		if !seen[tool.Name()] {
			seen[tool.Name()] = true
			out = append(out, tool)
		}
	}
	return out
}
