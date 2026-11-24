package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/intercept"
	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"
)

// Planner is the event-driven LLM planner (docs §4.3): each time the asset or
// exploration graph changes (debounced), it reads the exploration route, queries
// assets, judges whether the task goal is met, and emits 0..N exploration intents
// into the frontier. It is the sole intent generator.
type Planner struct {
	findingRecorder   FindingRecorder
	prov              llm.Provider
	model             string
	tx                *transcript.Store                      // raw LLM conversation persistence (nil = off)
	window            int                                    // context window in tokens (for compaction)
	windowFn          func() int                             // optional dynamic task-chain minimum
	maxTurns          int                                    // max agent turns per run (0 = unlimited)
	killWork          func(intentID int64) error             // engine callback to terminate a running work (nil = off)
	steerWork         func(intentID int64, msg string) error // engine callback to steer a running work mid-run (nil = off)
	proxyAddr         string                                 // recording proxy for WebFetch (empty = direct)
	proxyCACert       string                                 // recording proxy's CA cert path (HTTPS verify)
	webSearch         WebSearchOpts                          // web_search tool backend selection (off by default)
	workDir           string                                 // shared work dir (surfaced in prompt as artifact-output target)
	injectConstraints func() bool                            // resolver: inject task operation constraints into system prompt? (nil = yes)
	nonStreamingFn    func() bool                            // resolver: use non-streaming (Complete) path? (nil = streaming)
	noaEnabledFn      func() bool                            // resolver: use experimental noa compaction? (nil = off)
	maxTokensFn       func() int                             // resolver: per-reply output cap (nil/0 = send no cap)
	compactor         *Compactor                             // cold-node compaction (§7); nil = disabled

	// todos keeps ONE plan-scratchpad per task (keyed by exploration id) so the
	// planner's multi-step plan survives across wake-ups — each Plan() is a fresh
	// session, but the shared store lets it record a serial exploit chain once and
	// dispatch it step-by-step over rounds instead of front-loading it in parallel.
	todoMu sync.Mutex
	todos  map[int64]*actool.TodoStore
}

func NewPlanner(prov llm.Provider, model, workDir string, tx *transcript.Store, window, maxTurns int) *Planner {
	return &Planner{prov: prov, model: model, workDir: workDir, tx: tx, window: window, maxTurns: maxTurns, todos: map[int64]*actool.TodoStore{}}
}

func (p *Planner) SetCompactionWindowResolver(fn func() int) { p.windowFn = fn }

// SetCompactor wires the cold-node compactor (cold-digest §7). Called each
// planner wake-up to advance the round counter, maintain cold stamps, and
// (off the hot path) fold cold nodes into digests. nil = feature disabled.
func (p *Planner) SetCompactor(c *Compactor) { p.compactor = c }

// SetNonStreaming wires a resolver deciding whether runs use the non-streaming
// model path (true = non-streaming). nil/unset = streaming (default).
func (p *Planner) SetNonStreaming(fn func() bool) { p.nonStreamingFn = fn }

func (p *Planner) nonStreaming() bool { return p.nonStreamingFn != nil && p.nonStreamingFn() }

// SetNoaEnabled wires a resolver deciding whether runs use the experimental noa
// context-compression mechanism. nil/unset = off (built-in compaction). Read per
// run so the settings toggle takes effect without rebuilding the agent.
func (p *Planner) SetNoaEnabled(fn func() bool) { p.noaEnabledFn = fn }

// SetMaxTokens wires a resolver for the per-reply output cap. nil/unset or 0 =
// send no cap and let the endpoint decide. Read per run, like nonStreaming.
func (p *Planner) SetMaxTokens(fn func() int) { p.maxTokensFn = fn }

func (p *Planner) maxTokens() int {
	if p.maxTokensFn == nil {
		return 0
	}
	return p.maxTokensFn()
}

func (p *Planner) compactionWindow() int {
	if p.windowFn != nil {
		return p.windowFn()
	}
	return p.window
}

// SetProxy points the planner's WebFetch at the recording proxy plus the CA cert
// it trusts to verify HTTPS through it (empty addr = direct).
func (p *Planner) SetProxy(addr, caCert string) { p.proxyAddr, p.proxyCACert = addr, caCert }

// SetWebSearch selects the web_search backend for the planner (off by default).
func (p *Planner) SetWebSearch(o WebSearchOpts) { p.webSearch = o }

// SetConstraintInject wires a resolver deciding whether this task's operation
// constraints get injected into the planner system prompt. Read per round so the
// settings toggle takes effect without rebuilding the agent. nil = inject (default).
func (p *Planner) SetConstraintInject(fn func() bool) { p.injectConstraints = fn }

// wantConstraints reports whether constraint injection is enabled (default yes).
func (p *Planner) wantConstraints() bool { return p.injectConstraints == nil || p.injectConstraints() }

// todoFor returns the task's persistent planning todo store, creating it on first
// use. Shared across all of this task's planner wake-ups.
func (p *Planner) todoFor(expID int64) *actool.TodoStore {
	p.todoMu.Lock()
	defer p.todoMu.Unlock()
	s := p.todos[expID]
	if s == nil {
		s = actool.NewTodoStore()
		p.todos[expID] = s
	}
	return s
}

// SetKillWork wires the engine's per-work terminate callback so the planner's
// kill_work tool can stop a single running worker.
func (p *Planner) SetKillWork(fn func(intentID int64) error) { p.killWork = fn }

// SetSteerWork wires the engine's per-work steering callback so the planner's
// steer_work tool can inject a mid-run course-correction into a running worker.
func (p *Planner) SetSteerWork(fn func(intentID int64, msg string) error) { p.steerWork = fn }

// renderPlannerTodos formats the persistent planning todo for injection into the
// wake-up prompt (empty when there are no todos yet — first wake-up).
func renderPlannerTodos(items []actool.Todo) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n【Deine Planungsaufgaben (übergreifend bei Weckungen beibehalten, letzte Runde von dir geschrieben)】:\n")
	for _, it := range items {
		mark := map[actool.TodoStatus]string{actool.TodoPending: "☐", actool.TodoInProgress: "▶", actool.TodoCompleted: "✔"}[it.Status]
		if mark == "" {
			mark = "☐"
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", mark, it.Content))
	}
	b.WriteString("Daher vorgehen: Nur für den nächsten Schritt, dessen Voraussetzungen erfüllt sind / dessen abhängige Facts bereits existieren, Intents erteilen; mit TodoWrite die Liste aktualisieren (bereits durch Facts erfüllte Schritte als completed markieren). Keine im Plan noch pending/in_progress stehenden Schritte erneut erteilen.")
	return b.String()
}

// TriggerEvent describes what concretely caused this planning round to fire, so
// the planner looks first at the actual change instead of re-scanning the whole
// overview. Kind:
//
//	"done"    — a worker finished intent IntentID (its output conclusion is fetched).
//	"finding" — a worker reported a finding on intent IntentID (Detail = 摘要).
//	"goal"    — the human (via 主 agent 的 set_goals) added one OR MORE goals in a
//	            single call (Goals = 本次新增的目标文本，1+ 条；set_goals 支持批量).
//	"goal_deleted" — the human deleted a goal from 总览的目标管理 (Detail = 被删目标文本).
//	"goal_edited"  — the human edited a goal from 总览的目标管理 (OldGoal→NewGoal 文本).
//	"cancelled" — the human deleted intent IntentID (Detail = 删除原因). The intent is
//	            stopped (not deleted) and the reason is attached to it as a fact.
type TriggerEvent struct {
	Kind     string
	IntentID int64
	Detail   string
	Summary  string   // Kind=="cancelled" 专用：删除前捕获的意图摘要（真删除后节点已不存在，无法再查）
	Goals    []string // Kind=="goal" 专用：本次 set_goals 新增的目标文本（1 条或多条）
	OldGoal  string   // Kind=="goal_edited" 专用：修改前的目标文本
	NewGoal  string   // Kind=="goal_edited" 专用：修改后的目标文本
	Hints    []string // Kind=="hint" 专用：本次 add_hint 新增的提示文本（1 条或多条）
}

// renderTriggers spells out the change(s) that fired this round: for a finished
// worker — which intent + its output conclusion; for a finding — which intent +
// what was found. Empty for time/heartbeat wakes. Reads the store (best-effort;
// a blank field never blocks the round).
func renderTriggers(ts *db.ExplorationStore, evs []TriggerEvent) string {
	if len(evs) == 0 || ts == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n【Die tatsächlichen Änderungen, die diese Runde ausgelöst haben (huerst schauen, dann entscheiden, ob Richtung ergänzt werden soll)】:")
	for _, ev := range evs {
		switch ev.Kind {
		case "goal":
			if len(ev.Goals) == 1 {
				b.WriteString(fmt.Sprintf("\n- Mensch (Haupt-Agent) hat ein Ziel hinzugefügt: %s —— Neues zu erreichendes Ziel, bitte entsprechend Explorationsrichtungen ergänzen (falls noch kein entsprechender Intent existiert).", ev.Goals[0]))
			} else {
				b.WriteString(fmt.Sprintf("\n- Mensch (Haupt-Agent) hat %d Ziele hinzugefügt: %s —— Allesamt neue zu erreichende Ziele, bitte für jedes noch ohne entsprechenden Intent Explorationsrichtungen ergänzen.", len(ev.Goals), strings.Join(ev.Goals, ";")))
			}
		case "hint":
			if len(ev.Hints) == 1 {
				b.WriteString(fmt.Sprintf("\n- Mensch (Haupt-Agent) hat einen strategischen Hinweis hinzugefügt: %s —— Bereits im Explorationsgraphen hinterlegt, bitte entsprechend Explorationsrichtungen anpassen/ergänzen (falls noch kein entsprechender Intent existiert).", ev.Hints[0]))
			} else {
				b.WriteString(fmt.Sprintf("\n- Mensch (Haupt-Agent) hat %d strategische Hinweise hinzugefügt: %s —— Allesamt bereits im Explorationsgraphen hinterlegt, bitte jeweils entsprechend Explorationsrichtungen anpassen/ergänzen.", len(ev.Hints), strings.Join(ev.Hints, ";")))
			}
		case "goal_deleted":
			b.WriteString(fmt.Sprintf("\n- Mensch hat das Ziel gelöscht: %s —— Ziel wurde entfernt, bitte verbleibende Ziele/Richtungen neu bewerten (dafür keine Intents mehr erteilen).", ev.Detail))
		case "goal_edited":
			b.WriteString(fmt.Sprintf("\n- Mensch hat das Ziel geändert, von „%s“ zu „%s“ — bitte passe die Erkundungsrichtung gemäß dem neuen Ziel an (falls die ursprüngliche Richtung nicht mehr anwendbar ist, bitte einstellen).", ev.OldGoal, ev.NewGoal))
		case "finding":
			b.WriteString(fmt.Sprintf("\n- Intent #%d (%s) Worker hat einen Fund gemeldet: %s", ev.IntentID, intentSummary(ts, ev.IntentID), ev.Detail))
		case "cancelled":
			// 意图内容优先用删除时捕获的 Summary（真删除后节点已不存在，intentSummary 查不到）。
			sm := ev.Summary
			if sm == "" {
				sm = intentSummary(ts, ev.IntentID)
			}
			b.WriteString(fmt.Sprintf("\n- Intent #%d wurde vom Benutzer gelöscht, Intent-Inhalt: %s, Löschgrund: %s. Der Intent wurde gelöscht (nicht mehr ausgeführt); bitte entsprechend neu planen.", ev.IntentID, sm, ev.Detail))
		default: // "done"
			b.WriteString(fmt.Sprintf("\n- Intent #%d (%s) Worker beendet, Ausgabe-Fazit: %s", ev.IntentID, intentSummary(ts, ev.IntentID), workerOutput(ts, ev.IntentID)))
			if fids := factIDsYielded(ts, ev.IntentID); fids != "" {
				b.WriteString(fmt.Sprintf("; Dieser Intent neu erzeugte Fact-IDs: %s ", fids))
			}
		}
	}
	b.WriteString("\n(Vollständige Details per node_detail / get_worker_output / list_findings abrufbar.)")
	return b.String()
}

// factIDsYielded lists the fact ids an intent produced this run as "#12、#15", so the
// planner can jump straight to the round's incremental facts. Empty (best-effort) when
// the intent yielded no facts or the lookup fails.
func factIDsYielded(ts *db.ExplorationStore, id int64) string {
	ids, err := ts.FactsYielded(id)
	if err != nil || len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, fid := range ids {
		parts[i] = fmt.Sprintf("#%d", fid)
	}
	return strings.Join(parts, "、")
}

// intentSummary reads an intent node's one-line summary (best-effort, "?" on miss).
func intentSummary(ts *db.ExplorationStore, id int64) string {
	n, err := ts.GetNode(id)
	if err != nil || n == nil {
		return "?"
	}
	var p map[string]any
	if json.Unmarshal(n.Payload, &p) == nil {
		if s, ok := p["summary"].(string); ok && s != "" {
			return s
		}
	}
	return "?"
}

// workerOutput returns the finished worker's conclusion for an intent — the last
// 'result' (else 'text') activity's full detail, truncated. Same source get_worker_output uses.
func workerOutput(ts *db.ExplorationStore, id int64) string {
	acts, _, err := ts.ActivityList(&id, 0, 1000)
	if err != nil {
		return "(Ausgabe-Abruf fehlgeschlagen)"
	}
	var pick *db.Activity
	for i := range acts {
		if acts[i].Kind == "result" {
			pick = &acts[i]
		} else if acts[i].Kind == "text" && pick == nil {
			pick = &acts[i]
		}
	}
	if pick == nil {
		return "(Dieser Work hat noch keine Ausgaberecord)"
	}
	out, _ := ts.ActivityDetail(pick.ID)
	if out == "" {
		out = pick.Summary
	}
	return truncOutput(out, 800)
}

// truncOutput caps a worker-output blob so the trigger context doesn't bloat the
// system prompt every round; full text is one get_worker_output call away.
func truncOutput(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + " … (gekürzt, vollständig per get_worker_output)"
}

// renderGraphOverview folds the pre-computed graph_overview snapshot into the
// wake-up prompt so the planner starts each round with the full situation in
// hand — saving the round-trip it would otherwise spend calling the tool. It is
// the exact same JSON graph_overview would return; deeper detail is still one
// tool call away (node_detail / list_facts / …).
func renderGraphOverview(data map[string]any) string {
	b, err := json.Marshal(data)
	if err != nil {
		return "" // fall back to the model calling graph_overview itself
	}
	return "\n\n【Dieser Runde Lagebild (graph_overview vorab geholt, entspricht dem Return beim Selbstaufruf; für Details bei Bedarf node_detail/list_facts etc. aufrufen)】:\n" + string(b)
}

// plannerDefaultTmpl is the built-in EDITABLE body (段 [A]) of the planner prompt,
// seeded into agent_prompts. Goal is a {{.Goal}} template var; the 中间产物输出规约
// tail is code-owned (artifactSpec) and appended by plannerSystem after rendering.
const plannerDefaultTmpl = `Du bist der "Planer" eines autorisierten Penetrationstest-Systems einer Netzwerksicherheitsplattform und wirst häufig geweckt (bei jeder Graph-Änderung). Aufgabe: Lage lesen → Ziele beurteilen → **nur wenn es wirklich neue, nicht abgedeckte Richtungen gibt** zusätzliche Erkundungsabsichten ergänzen. Du bist Planer, nicht Ausführer: Alle Ergebnisse dieser Runde dürfen nur 【Absichten erzeugen/klären】 oder 【Ziele beurteilen】 sein, erledige im Plan niemals selbst die Arbeit.

Aufgabenziel: {{.Goal}}

**本轮该产出几个意图（先想清楚这条）**：
- **Harte Untergrenze (höchste Priorität)**: Solange 【Ziel nicht erreicht】 und 【derzeit keinerlei open oder running Intents】 (frontier_open=0 und running_intents leer) vorliegen, **muss** in dieser Runde mindestens ein das Ziel voranbringender Intent erzeugt werden — keine laufenden Works zum Warten, keine queued Richtungen; 0 Intents erzeugen = Task-Stillstand; selbst wenn bekannte Richtungen nur in recent_done stehen, gemäß unten done/exhausted/blocked-Beurteilung eine neue eröffnen oder fortsetzen.
- Abseits der harten Untergrenze ist **0 Intents erzeugen ein normales Ergebnis, aber mit berechtigtem Grund** (nicht Standard "weniger erzeugen = stabiler"): ① **Bereits abgedeckt** — alle dir einfallenden Richtungen werden bereits von noch open/running Intents bearbeitet (Wortlaut ändern und bestehende Intents duplizieren ist schwerer Fehler); ② **Warten auf Abhängigkeit** — nächster Schritt hängt von Ausgabe eines gerade laufenden Works ab, das noch nicht vorliegt (jetzt erzwingen würde Downstream ohne Vorlage leerlaufen lassen, soll nächste Weckung nach Graph-Update abwarten).
- Umgekehrt: Wenn es tatsächlich 【unabgedeckte, nicht von laufenden Works abhängige】 neue Richtungen gibt, oder Ziel unerreicht und im Scope noch ungeprüfte Flächen bestehen, dann muss erteilt werden — 0 Intents nicht als Faulheits-Default missbrauchen.

**每次唤醒的决策流程**：

1. **Vollständiges Lagebild ist unten im Prompt angehängt** (ist der graph_overview-Return, kein erneuter Aufruf nötig): task (Originaltitel+Ziel/Wurzelknoten), Asset-Zahl, goals+Status, open/running/recent_done Intents, sites_without_endpoints (Endpunktlose Sites, Hinweis auf mögliche noch zu erkundende Richtungen), facts (Anzahl Explorationsfacts, andere Kategorie als Findings), recent_facts ({id,summary,confidence?}).
   - **Scope**: Explorationsknoten (goals/Intents/facts/findings) enthalten nur den aktuellen Task; **Asset-Graph global geteilt** (mehrere Tasks dieselbe Instanz, Asset-Zahl ist global im Scope, nicht task-exklusiv) — bei Assets, die nicht zum aktuellen Task gehören, ignorieren.
   - **Herkunft**: Jeder Intent trägt parents (Upstream: abgeleitet aus welchen Facts/Intents) und yields (Downstream: welche Facts/Findings erzeugt), recent_facts je Eintrag with from_intent; daraus verstehen "welche Facts stammen aus welcher Richtung, lassen sich neue Richtungen synthetisieren".
   - **Negative/unsichere Beobachtungen** (in recent_facts "Port geschlossen/nicht injizierbar" etc.) sind Worker-Beobachtungen, keine Feststellungen: Vor Glauben node_detail(id) prüfen auf Evidence — nur wenn Evidence solide, confidence=observed und Methoden erschöpft, Richtung als vorläufig blockiert betrachten; Evidence fehlt, nur "sieht so aus/nur einmal getestet" oder confidence=inferred → als 【noch nicht geklärt** behandeln, falls im Scope und keine andere Intent abdeckt, standardmäßig einen Verifikations-Intent entsenden zum Belegen oder Widerlegen (**dieselbe negative Richtung max. einmal verifizieren**; Verifikation ergibt weiterhin negativ mit vernünftigem Evidence, dann Schlussfolgerung respektieren, nicht mehr entsenden).
   - **Für tiefere Details erst bei Bedarf aufrufen**: list_facts (paginiert, neueste zuerst, Default 20, q Filter, before Paginierung, mit total/has_more), list_findings (alle Findings), node_detail(id) (vollständige Evidenz/Details; Liste/recent_facts geben nur Summary), list_assets (pull: q Suche, type/company_id/task_id Filter, Paginierung, oder id/ids direkt), asset_neighbors. Assets global geteilt, nicht standardmäßig Vollmenge ziehen.

2. **Ziele beurteilen (Kernaufgabe)**: goals-Feld enthält bereits Ziele mit Status; für durch Facts/Findings belegte, noch unerreichte Ziele prove_goal(goal_id, evidence_id, reason) aufrufen und als met markieren. **Wenn du genau das letzte unerledigte Ziel als met markierst, stuft das System den gesamten Task automatisch als abgeschlossen ein** — Abschluss nur durch jeweiliges prove_goal getrieben, kein anderer "One-Click-Fertig"-Mechanismus.
   - ⚠️ **Quantitative Abnahmeprüfung (vorzeitiges Abnicken verboten)**: Enthält Ziel quantifizierbare Bedingungen (Coverage X%, N Flags ergattern, bestimmte Berechtigung erlangen), vor prove_goal **zwingend** oben im graph_overview die Ist-Werte prüfen (coverage.pct, findings_total Count etc.): Nicht erreicht → **prove_goal verboten**, stattdessen Intents zur Lückenschließung entsenden; nicht mit "im Großen erreicht/Kern schon erledigt" vorzeitig met setzen. Bsp.: Coverage 100% gefordert, Ist coverage.pct=40% → nicht erreicht, weiter Nachtest-Intents entsenden.

3. ** (Optional, nur Start, extrem leicht) Verständnis sondieren**: Nur wenn Graph quasi noch keine Facts hat (recent_facts quasi leer, Task gerade gestartet), und allein aus Lagebild initiale Intents nicht konkretisierbar sind, darf mit Bash etc. extrem wenig, read-only am Ziel sondiert werden (z. B. 1–2 curl auf Startseite/Fingerprint). **Einzig legales Artefakt ist eine präzisere Intent-Beschreibung** — keinesfalls Vulnerability-Findung/Verifikation/Exploitation, auch keine Endpunkt/Verzeichnis/Parameter-Aufzählung (das ist Worker-Arbeit, als Intent formulieren und delegieren). Drei harte Grenzen:
   - Graph hat bereits Worker-produzierte Facts (facts>0 / recent_facts nicht leer) → **selbst sondieren verboten**, alle Urteile basieren auf bestehenden Facts, Runden-Artefakte nur "neue Intents entsenden" oder "beenden"; bestimmte Spur tiefer prüfen wollen → Intent entsenden, Worker prüfen lassen, nicht selbst curl.
   - Selbst am Start max. ≤3 Sonden dann stoppen, nur um initiale Intent zu klären; sobald man merkt, man betreibt "tiefergehende Verifikation" statt "schnell Richtung festlegen" (einzeln Endpunkte/Verzeichnisse aufzählen, IDs durchprobieren, Decode-Ketten, dieselbe Schnittstelle wiederholt sondieren, jegliche Injection/Privilegieneskalation/Vuln-Testverifikation — alles Worker-Schwerarbeit), sofort aufhören und als Intent formulieren.
   - Was aus bestehenden Facts/Lagebild beurteilbar ist, braucht gar keine Sonde.

4. **Entscheiden, welche neuen Richtungen zu ergänzen**: **Hier bedeutet "Zurückhaltung" nur 【keine bestehenden Intents duplizieren】, nicht "so wenig wie möglich entsenden"** — bei unerreichtem Ziel Standardfrage: "Welche tieferen, härteren, noch unabgedeckten Angriffswege bringen uns dem Ziel näher", nicht "können wir abschließen". Intents sind **offene Explorationsrichtungen** (keine festen Typen/Menüs), aus bekannten Facts, Assets, Zielen selbst Richtungen ableiten, jede mit open + running + recent_done abgleichen:
   - Bereits durch open/running abgedeckt → nicht neu erzeugen (wird bearbeitet).
   - In recent_done aufgetaucht → **zuerst den Intent-State (jede Eintrag hat einen) prüfen, wie er stoppte, dann entscheiden**:
          · **done (normal abgeschlossen)**: bereits abgedeckt → nicht unverändert neu vergeben; ob Sackgasse, zeigt sich am fact-Ergebnis aus yields, nicht am state; nur bei 【materiell neuem Mechanismus】 (neue Fakten/Assets/Parameter/deutlich andere Vorgehensweise) neu vergeben, und summary klar den Unterschied zum letzten Mal nennen; Umformulieren oder "vielleicht klappt es beim nächsten Versuch" zählt nicht, Wiederholung verboten.
          · **exhausted (Budget erschöpft, auf halbem Weg abgeschnitten, nur Teil zurückgeschrieben) / blocked (Modell- oder Netzwerkfehler, kaum erkundet)**: beide wurden mittendrin nicht sauber beendet, Infos unvollständig — zuerst mit get_worker_trace / get_worker_output sehen, wie weit es kam und wo es hängt, dann aus folgenden wählen: kurz vor dem Durchbruch vom Budget gestoppt → "bei letztem Fortschritt fortsetzen" vergeben; reiner externer Fehler ohne Erfolg (oft blocked) → dieselbe Richtung direkt neu vergeben; immer an derselben Stelle hängend → Vorgehen/Richtung wechseln. Grundlage ist immer der echte Fortschritt im trace, nicht der state selbst.
   - Völlig ohne jegliche Intent-Abdeckung völlig neue Richtung → erzeugen.
   - Alle bekannten Richtungen durch noch open/running Intents abgedeckt → nicht erzeugen, direkt beenden (Works laufen/queued, warten auf deren Fortschritt); aber wenn nur recent_done abdeckt, kein open/running mehr und Ziel unerreicht → gemäß harter Untergrenze oben neue eröffnen oder fortsetzen.
   - **Tiefe vor Coverage**: Coverage ist Untergrenze/Abnahmekriterium, nicht Explorationsziel an sich; hochwertigen Einstieg entdeckt (potenziell führend zu RCE/PrivEsc/Datenexfil), priorisiere Intents, um genau diesen Pfad **tief durchzustoßen**, statt für Coverage-Ausgleich breit, asset-weise flach zu testen.
   - **Routenvielfalt bewahren, nicht zu früh konvergieren**: Ziel unerreicht, wenn alle bestehenden Intents in derselben Route/Einstieg drängen, aber **wesentlich verschiedene** unabgedeckte Richtungen existieren (anderer Einstiegspunkt/andere Asset-Klasse/andere Exploit-Kette), priorisiere diese Abzweigung, nicht synonymen Intent auf derselben Linie (auf materielle Unterschiede schauen, nicht Wortlaut); wird die Abzweigung bereits von bestehendem Intent abgedeckt, trotzdem nicht erzeugen. Ideal: 2–3 mechanistisch verschiedene Routen parallel (z. B. "über Upload-Kette" vs. "über Auth-Bypass"), erst wenn eine Richtung **Zielannäherung** beweist, Ressourcen dorthin konzentrieren. **Aber Vielfalt untersteht immer den oberen [Operationsbeschränkungen]**: Durch Constraints ausgeschlossene Einstiegspunkte/Ports/Hosts/Operationen, auch wenn wesentlich anders, **niemals** Intents erzeugen.

   **串行利用链：分步派，别拆成并行。** 强依赖串行链（①→②→③，后一步依赖前一步的实际产出）：不要一次性并行下发（下游拿不到还不存在的前置只会重复/空转）；用 TodoWrite 把整条链记成待办（每步一条），本轮只派"前置已满足"的那步（通常第一步），待它产出 fact 后下次唤醒（提示会带上待办清单）再派下一步并把已满足的标 completed。"同一件事"别拆两条（"确认触发点"和"触发触发点"是同一步）；只有【平行、互不依赖】的维度（如枚举多个不相关端点）才用多意图并行。

5. **Einreichen**: Mit **einem** add_intent die gefilterten neuen Richtungen gebündelt einreichen (intents Array, max. 4 höchste Wert, nicht einzeln mehrfach aufrufen):
   - **summary**: Ein-Satz-Natursprache Beschreibung der Richtung (Testziel vollständige Adresse + was tun + warum), kein festes Schema; Deduplizierung hauptsächlich durch Abgleich mit bestehenden Intents.
   - **asset_ids**: Ziel-Asset-IDs für Test/Angriff dieser Richtung (möglichst angeben, 0/1/mehrere, aus list_assets) — sobald Richtung um konkrete Assets (Site/Interface/Parameter/Host) kreist, zwingend angeben, für Coverage-Dedupe und Asset-Verknüpfung, bei mehreren Assets alle angeben; nur bei rein globaler Rekognoszierung ohne konkretes Asset leer lassen.
   - **parent_ids**: Aus welchen Upstream-Knoten diese Richtung synthetisiert wurde (optional, 0/1/mehrere) — mehrere Facts kombiniert zu einem Intent → alle angeben, abgeleitet von Upstream-Intent/Finding → dessen ID angeben, völlig neue Top-Level-Richtung → leer lassen.

Keine Duplikate, nicht krampfhaft auffüllen; aber Ziel unerreicht und tieferliegende unabgedeckte Angriffswege vorhanden → entsenden. Knapp, fokussiert, effizient.`

func plannerSystem(goal, dataDir, workDir string) string {
	body := renderSystem("planner", plannerDefaultTmpl, PlannerVars{Goal: goal, DataDir: dataDir, Now: nowStr()})
	return body + artifactSpec(workDir)
}

// Plan runs one planning round. emit, if non-nil, receives the planner's execution
// steps (so users can see how it reads the situation and judges goals — the
// planner is the intent generator and was previously a black box). Returns whether
// the planner judged the goal met.
// triggers carries the concrete change(s) that fired this round — worker(s) done
// and/or finding(s) reported (may be several — the engine debounces a burst; empty
// for time/heartbeat wakes). They are spelled out at the top of the prompt so the
// planner looks first at the actual change (which intent, its output/finding).
func (p *Planner) Plan(ctx context.Context, taskID int64, as *db.AssetStore, ts *db.ExplorationStore, goal string, triggers []TriggerEvent, emit func(db.Activity)) (met bool, reason string, err error) {
	// cold-digest §2.3/§7: advance this task's planner-round counter, maintain the
	// cold_since_round stamps, and (if a threshold is hit) kick off background
	// compaction. Synchronous part is cheap (a few queries); the LLM compaction
	// runs in a detached goroutine so it never adds latency to this round.
	p.compactor.OnPlannerRound(ctx, ts)
	tsx := NewToolSet(ts, "planner")
	tsx.SetFindingRecorder(p.findingRecorder)
	if as != nil {
		tsx.SetAssetStore(as, as.Companies())
	}
	tsx.SetTaskID(taskID)
	tsx.SetCoverageEnabled(as == nil || as.CoverageEnabled(taskID))
	tsx.killWork = p.killWork   // enable kill_work tool (nil = unavailable)
	tsx.steerWork = p.steerWork // enable steer_work tool (nil = unavailable)
	if origin, _ := ts.OriginFactID(); origin > 0 {
		tsx.SetOwnerNode(origin) // planner-side anchors default to the task root (origin fact)
	}
	// 领域工具 + 基础默认工具集（Read/Write/Edit/MultiEdit/LS/Glob/Grep/Bash）
	// 资产覆盖度功能关闭时剔除 add_task_scope/list_untested_assets（不入 prompt）。
	base := append(tsx.DropCoverageTools(tsx.PlannerTools()), actool.DefaultTools()...)
	ctx = WithRunInfo(ctx, RunInfo{TaskID: taskID, ExplorationID: explorationID(ts)})
	tools, def, cleanup := AugmentTools(ctx, "planner", base)
	defer cleanup()
	// 关键态势（刚完成的意图 + 预取的完整图）改放【本轮 user 输入】(见下方 input)，system
	// 只留静态规划正文。move-out 让 system 每轮稳定、更利于缓存；代价是若单轮变长，态势可能
	// 被 compaction 压缩（planner 单轮通常短，风险低）。situational 会拼进下方 input。
	situational := renderTriggers(ts, triggers) + renderGraphOverview(tsx.graphOverviewData())
	// 任务级 deadline / 终局模式(经 ctx 注入,见 taskclock.go)。终局那一轮把任务超时
	// planner 收尾词作为【本轮操作指令】拼进本轮 user 输入(随 situational),让它只做最后
	// 目标判定、不产新意图。
	tc := taskClockFrom(ctx)
	if tc.Final {
		situational += "\n\n【Aufgabenendabwicklung (dieser Runde Spezialanweisung, überschreibt obigen regulären Planungsablauf)】:" + resolveTaskTimeoutWrapup("planner")
	}
	// 本任务的工作目录 <workDir>/tasks/<taskID>，先建好。
	taskDir := ensureRunDir(p.workDir, taskID, 0)
	ctx = intercept.WithReviewContext(ctx, taskDir, intercept.ReviewBackground{})
	sysBody := plannerSystem(goal, p.workDir, taskDir)
	if p.wantConstraints() {
		sysBody += constraintBlock(ts) // 操作约束(若有)注入系统提示,框定探索边界
	}
	system, boundary := deferredSystem(sysBody, def)
	// planner 无自身墙钟预算;有 deadline 时把 MaxDuration 夹逼到剩余,让在跑的规划轮在
	// 任务到点时进收尾(因超时→任务超时词,因步数→per-run 词)。
	maxDur, clamped := clampMaxDuration(tc.DeadlineUnix, 0)
	settle := wrapupSettlement("planner", nil)
	if tc.DeadlineUnix > 0 {
		settle = wrapupSettlementForTask("planner", nil, clamped)
	}
	opts := agentcore.Options{
		Provider:        p.prov,
		SystemPrompt:    system,
		DynamicBoundary: boundary,
		Tools:           tools,
		DeferredTools:   def.Deferred,
		UnlockSet:       def.Unlock,
		PermissionMode:  permission.ModeBypass,
		EnableWebFetch:  true, // 走记录代理留痕；载入代理 CA 验证 MITM 重签的 HTTPS 证书
		WebFetchProxy:   p.proxyAddr,
		WebFetchCACert:  p.proxyCACert,
		// 联网搜索(可选)。ddgs 无需 key；brave-free 需 BraveKey；tavily 需 TavilyKey。
		// WebSearchProxy 是独立出口代理(http/https/socks5)，与记录流量的 MITM 代理无关；空则直连。
		EnableWebSearch:       p.webSearch.Enabled,
		WebSearchBackend:      p.webSearch.Backend,
		BraveSearchAPIKey:     p.webSearch.BraveKey,
		TavilySearchAPIKey:    p.webSearch.TavilyKey,
		DeepSeekSearchBaseURL: p.webSearch.DeepSeekBaseURL,
		DeepSeekSearchAPIKey:  p.webSearch.DeepSeekAPIKey,
		DeepSeekSearchModel:   p.webSearch.DeepSeekModel,
		WebSearchProxy:        p.webSearch.Proxy,
		BashEnv:               proxyEnv(p.proxyAddr, p.proxyCACert), // Bash 子命令默认走代理+信任 CA
		WorkingDir:            taskDir,                              // 本任务工作目录 <workDir>/tasks/<taskID>
		ToolOutputDir:         cmdOutDir(taskDir),
		MaxTurns:              p.maxTurns, // 0 = unlimited (configurable in agent management)
		MaxDuration:           maxDur,     // 0=不限;有 deadline 时=距 deadline 剩余
		Compaction:            compactionConfig(p.compactionWindow()),
		// 跨唤醒共享的规划待办：让串行链在多轮之间保留（session 是新的，store 不是）。
		Todos: p.todoFor(ts.ID()),
		// 命中【本轮】步数预算→ SDK 跑收尾:把本轮已想清楚的结论落地(该派的 add_intent、
		// 能证的 prove_goal、串行链记 TodoWrite),而非停止规划——planner 之后仍会被反复唤醒。
		// clamped(被任务 deadline 夹逼)时改用 PromptByReason(见 wrapupSettlementForTask)。
		Settlement:   settle,
		NonStreaming: p.nonStreaming(), // 该 profile 选非流式时走 Provider.Complete
		MaxTokens:    p.maxTokens(),    // 0 = 不发上限,由服务端默认值决定
	}
	if p.tx != nil { // persist raw LLM conversation; one accumulating file per task's planner
		opts.Transcript = p.tx
		opts.SessionID = fmt.Sprintf("exp%d-planner", ts.ID())
	}
	// 实验功能:开启后由 noa 接管上下文压缩(归档集中在 <workDir>/noa/<SessionID> 下,持久)。
	noaSession := fmt.Sprintf("exp%d-planner", ts.ID())
	enableNoa(&opts, p.noaEnabledFn, p.workDir, noaSession, noaWarn(noaSession))
	// 态势（刚完成的意图 + 完整图）现在拼进本轮 user 输入（见下方 input）。user 里还有
	// 指令 + 跨唤醒待办（todo 是模型自己的规划便签，可再生，放 user 即可）。
	// 开场白按「本轮有无具体变动」分两种：有变动 → 指向下方【实际变动】块；无变动
	// (心跳定时巡检 / hint / 恢复等) → 别谎称"图发生了变化",转而提示顺带复查在跑意图。
	lead := "Es gab gerade konkrete Änderungen (siehe unten 【Die tatsächlichen Änderungen, die diese Runde ausgelöst haben】), plane entsprechend den nächsten Schritt:"
	if len(triggers) == 0 {
		lead = "Diese Runde ist **planmäßige Patrouille (Heartbeat fällig)/ohne konkretes Veränderungssignal** geweckt — Graph muss nicht zwingend neue Änderungen haben. Nebensächlich laufende Intents prüfen: lange ohne Fortschritt oder abgedriftete per steer_work korrigieren, grundlegend falsche Richtung per kill_work stoppen; dann Ziele beurteilen, entscheiden ob Richtungen ergänzen:"
		// 心跳/无变动唤醒时,若全图已无任何 open 或 running 意图 → 探索已停摆(没 worker 在跑、
		// 也没排队方向)。明确告知 planner 并强制其本轮补出新方向,别只复查在跑意图后空转一轮。
		if active, err := ts.HasActiveIntent(); err == nil && !active {
			lead = "Diese Runde ist **planmäßige Patrouille (Heartbeat fällig)** geweckt, und aktuell **existieren keinerlei open oder running Intents** — keine Worker laufen, keine queued Richtungen, Exploration steht still. Du **musst** in dieser Runde einen oder mehrere das Ziel voranbringende, mit Graphenbestand **nicht-duplizierende** neue Intents erzeugen (0 Intents unzulässig); erst anhand untenstehendem Lagebild beurteilen ob Ziel bereits erreicht, falls nicht sofort Richtungen ergänzen:"
		}
	}
	input := lead + situational + "\n\nAuf Basis obigen Lagebilds Ziele beurteilen. Ziele **echt erreicht** (Zielergebnis bereits in Händen / Zielvulnerability bestätigt) → prove_goal je einzeln markieren. **Harte Untergrenze: Solange Ziel noch nicht erreicht und derzeit keinerlei open oder running Intents (frontier_open=0 und running_intents leer) vorliegen, muss diese Runde mindestens einen das Ziel voranbringenden Intent erzeugen — keine laufenden Works zum Warten, keine queued Richtungen, 0 Intents = Task-Stillstand. Nur wenn bereits open/running Intents vorankommen oder Ziel schon erreicht, darf diese Runde auf neue Intents verzichten.**" +
		renderPlannerTodos(opts.Todos.List())
	// MaxDuration 现在会在墙钟到点打断在跑工具并就地进收尾(在活 ctx 上),单轮卡死不再
	// 绕过收尾,无需外部硬 ctx 兜底。ctx 只承载 pause / kill / shutdown。
	_, _, err = captureRun(ctx, opts, input,
		func(r db.Activity) {
			if emit != nil {
				r.Worker = "planner" // planner activity has no intent_id (it generates them)
				emit(r)
			}
		})
	return tsx.GoalMet, tsx.Reason, err
}
