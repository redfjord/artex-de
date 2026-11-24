package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/norma/agentcore"
	"github.com/Autumn-27/norma/llm"
	acperm "github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
	"github.com/Autumn-27/norma/transcript"
)

// goalsDefaultTmpl is the built-in EDITABLE body (段 [A]) of the goals-decomposer
// prompt, seeded into agent_prompts. No template vars are used today.
const goalsDefaultTmpl = `Du bist ein Ziel-Zerleger für Penetrationstests. Deine Aufgabe ist es, aus der Benutzereingabe das **endgültig zu erreichende Ergebnis** zu identifizieren, nicht die Angriffsschritte zu planen.

**第一步（拆分目标之前先做）：抽取操作约束**
Identifiziere aus „Aufgabenziel / Aufgabenbeschreibung“ die expliziten Vorgaben des Operators zu 【was getan werden darf, was nicht getan werden darf】, registriere sie zeilenweise via set_constraints (falls Beschreibung/Ziel keine operativen Constraints enthalten, kann die Constraint-Extraktion unterbleiben):
- type=deny: Verbotene Operationen (z. B. „kein Portscan“, „keine Schreib-/Leseoperationen in Produktion“, „Brute-Force verboten“, „bestimmte Subdomain nicht anfassen“).
- type=allow: Explizit erlaubter/begrenzter Operationsbereich (z. B. „nur passive Aufklärung“, „nur gegen bestimmte Domain“).
- Constraint ≠ Ziel, auch ≠ Angriffsstep: Es ist eine Festlegung der operativen Verhaltensgrenzen.
- **Constraints müssen 【selbstständig, konkrete Ziele hardcoded】 sein**: Ersetze **Pronomen** wie „aktuelles Ziel/aktueller Port/aktuelle IP/aktuelle Domain/diese Site“ durch die **konkreten Werte** aus Aufgabenziel/Beschreibung. Constraints werden separat in die Ausführungs-Prompts injiziert; ohne Kontext lassen sich Pronomen nicht auflösen.
  Beispiel: Ziel ist https://abc.example.net → schreibe „nur Test von abc.example.net erlaubt“ statt „nur Test des aktuellen Ziels erlaubt“; „nur Zielport 443 testen, keine anderen Ports scannen“ statt „nur aktuellen Port testen“. Wenn der Originaltext nur „aktuelles Ziel“ sagt, die Zieladresse aber explizit genannt ist, füge die Adresse ein.
- **Nur Constraints registrieren, die im Ziel/Beschreibung 【explizit geschrieben oder betont】 sind, strikt verboten zu erfinden**; bei unsicherem Typ deny verwenden (konservativer).
- Falls Ziel/Beschreibung tatsächlich keine operativen Constraints enthalten, **nicht** set_constraints aufrufen.
Nach Constraint-Registrierung (falls vorhanden) fortfahren mit der Ziel-Zerlegung unten.

**目标 = 最终可交付/可核验的结果**

**不是目标的内容（禁止列为子目标）**：
- Informationssammlung, Aufklärung, Endpunkt-Scan
- Schwachstellenanalyse und Verifikationsprozess
- Angriffssteps, Exploit-Methoden
- Ergebnis-Verifikationsschritte

**拆分原则**：
- Vom Benutzer beschriebenes Endziel ist nur eines → ein Ziel ausgeben
- Existieren mehrere **voneinander unabhängige** finale Deliverables → jeweils auflisten
- Wenn einer klaren Schwachstellenklasse zuordenbar, vulnclass annotieren; Informationssammlungs-/Business-Logic-Ziele leer lassen
- Strikt verboten, vom Benutzer nicht genannte Ziele zu erfinden

set_goals aufrufen, um Ergebnisse zu übermitteln.`

// goalsScopeTail is the code-owned tail appended after the editable goals body
// WHEN an asset store + task context are available. It teaches the decomposer to
// also lift the explicit asset scope out of the goal/description and register it
// via add_task_scope. Kept in code (not the DB-editable body) so it always applies
// on released DBs and can't be edited away — same pattern as the trafficTool tail.
const goalsScopeTail = `

**额外职责：登记测试资产范围**
Neben der Zielzerlegung musst du aus „Aufgabenziel / Aufgabenbeschreibung“ den **explizit angegebenen Test-Asset-Bereich** identifizieren und via add_task_scope registrieren (Autorisierungsgrenze dieser Aufgabe, auch Nenner für Asset-Testabdeckung). **Minimalbereichsprinzip: Nur das eine Ziel registrieren, das der Benutzer explizit angibt, keinesfalls eigenmächtig erweitern.**
- Ist das Ziel eine URL oder ein Hostname-basierte Adresse (z. B. https://xxx.example.com/path, app.example.com) → nimm den **vollständigen Hostnamen**, kind=subdomain, value=vollständiger Hostname.
  Beispiel: Ziel https://a1b2c3.lab.example.net/path → kind=subdomain, value=a1b2c3.lab.example.net (**nicht** example.net).
  **严禁**把带子域的主机名缩成根域名——看到 xxx.example.com 就登记整个 example.com 会把范围扩到用户目标之外，违背最小范围原则。
- Nur wenn der Benutzer **bloße Root-Domain ohne jegliche Subdomain** angibt (z. B. direkt example.com) oder explizit „gesamte Site / alle Subdomains / alle Domains“ sagt → erst dann kind=root_domain, value=example.com verwenden.
- Reine IP oder Netzsegment → kind=ip / cidr, value=IP oder CIDR.
- **Nicht** Firmenbereich (company) registrieren — Aufgabe gerade erstellt, Assetsystem hat die Firma meist noch nicht, Registrierung schlägt fehl, Firmenebene wird in nachfolgender Plan-Phase behandelt.
Weitere Regeln:
- Nur **im Ziel/Beschreibung explizit geschriebene** Bereiche registrieren; strikt verboten, nicht erwähnte Domains/IPs zu erfinden oder abzuleiten.
- reason kurz begründen, aus welchem Satz die Angabe stammt, für Audit-Zwecke.
- Falls Ziel/Beschreibung keinen expliziten Asset-Bereich enthalten, **nicht** add_task_scope aufrufen.
Zuerst mit add_task_scope Bereich registrieren (falls vorhanden), dann set_goals aufrufen, um Ziele zu übermitteln.`

// GoalSpec is one decomposed objective.
type GoalSpec struct {
	Text      string `json:"text"`
	VulnClass string `json:"vulnclass,omitempty"`
}

// DecomposeGoals asks the LLM to break a pentest task goal into discrete,
// independently-verifiable objectives (each becomes a goal node). Returns nil if
// no provider is configured or the call yields nothing — the caller then falls
// back to a rule-based split so goal nodes always exist.
//
// prov is supplied by the caller (rather than built here from a Config) so goal
// decomposition rides the SAME provider instance as the rest of the engine — it
// shares the rate limiter, gets recorded by llmrec, and participates in LLM
// failover instead of quietly bypassing all three.
//
// desc is the task's free-text description (背景：靶标范围/flag 数量/交战说明等).
// It is fed alongside the goal so the decomposer no longer splits blind — the
// prompt still forbids inventing anything the two texts don't state.
//
// emit, when non-nil, receives every LLM step (thinking/tool_use/result) with
// Worker="planner" so the round-0 goal-decomposition activity is visible in the UI.
//
// as + taskID, when non-nil/positive, wire the add_task_scope tool so the
// decomposer can register the explicit asset scope it extracts from the goal.
//
// ts is the task's exploration store: set_goals writes the decomposed goal nodes
// straight into it (the same managed tool the main agent uses to add goals at
// runtime). The returned specs are read back from the store so callers can emit
// per-goal activity and detect the "LLM produced nothing" case for their fallback.
func DecomposeGoals(ctx context.Context, prov llm.Provider, dataDir, goalText, desc string, as *db.AssetStore, ts *db.ExplorationStore, taskID int64, emit func(db.Activity)) []GoalSpec {
	if prov == nil {
		return nil
	}
	return DecomposeGoalsWithProvider(ctx, prov, dataDir, goalText, desc, as, ts, taskID, false, 0, emit)
}

// DecomposeGoalsWithProvider is the task-runtime variant used when a task has an
// ordered provider chain. It preserves the same tools and write behavior while
// letting the caller own provider selection/failover. maxTokens is the profile's
// per-reply output cap (0 = send none).
func DecomposeGoalsWithProvider(ctx context.Context, prov llm.Provider, dataDir, goalText, desc string, as *db.AssetStore, ts *db.ExplorationStore, taskID int64, nonStreaming bool, maxTokens int, emit func(db.Activity)) []GoalSpec {
	if prov == nil {
		return nil
	}
	// 目标拆解是一次性调用：不挂 transcript store，所以 agentcore 不会往 ctx 上挂
	// session id（它只在有 writer 时才挂，见 agentcore.Prompt）。而按 session-id 头
	// 做提示缓存/粘性路由的网关（opencode zen 缺 x-opencode-session 直接 400
	// MissingSessionID）读的就是 ctx 上这个值——不补就是「对话正常、拆解 400」。
	// 显式挂一个稳定 id：同一探索的拆解请求共享它（利于命中缓存），且命名与
	// planner/worker 不冲突，能被 llmrec.parseSession 正确归因。
	if ts != nil {
		ctx = transcript.WithSessionID(ctx, fmt.Sprintf("exp%d-goals", ts.ID()))
	}
	// worker="goals" tags the goal nodes' provenance; ts/taskID let set_goals link
	// each goal under the task root. This is the catalog's real set_goals tool, so a
	// web-edited description/schema on it applies here too.
	tsx := &ToolSet{as: as, ts: ts, taskID: taskID, worker: "goals"}
	// Description rides in the user message (same channel as the goal), NOT via the
	// {{.EngagementDescription}} template var — else a prompt that references the var
	// would inject the description twice. System prompt stays pure static instructions.
	sys := renderSystem("goals", goalsDefaultTmpl, GoalsVars{DataDir: dataDir, Now: nowStr()})
	// set_constraints 始终可用(不依赖 asset store):正文已含「先抽操作约束再拆目标」这步
	// (可在 agent 编辑页改措辞),这里只需接上工具。
	tools := []actool.CoreTool{tsx.setGoals(), tsx.setConstraints()}
	// Wire add_task_scope only when we have a real asset store + task to write to.
	// The scope-extraction tail is appended in lockstep so the prompt never asks for
	// a tool that isn't present.
	if as != nil && taskID > 0 {
		tools = append(tools, tsx.addTaskScope())
		sys += goalsScopeTail
	}
	userMsg := "Aufgabenziel:\n" + goalText
	if d := strings.TrimSpace(desc); d != "" {
		userMsg += "\n\nAufgabenbeschreibung (Hintergrundinfo, kann Zielbereich/Flag-Anzahl/Engagement-Regeln enthalten; nur Referenz, darin nicht Erwähntes nicht erfinden):\n" + d
	}
	// Use captureRun so every LLM step is emitted as an activity record (visible in
	// the plan tab under the round-0 marker). Falls back gracefully when emit is nil.
	captureEmit := func(r db.Activity) {
		if emit != nil {
			r.Worker = "planner"
			emit(r)
		}
	}
	captureRun(ctx, agentcore.Options{
		Provider:               prov,
		SystemPrompt:           []string{sys},
		Tools:                  tools,
		PermissionMode:         acperm.ModeBypass,
		DisableBackgroundTasks: true,
		// 3 步(抽约束 → 登记范围 → 拆目标)各需一次工具调用,给足回合避免收尾前漏调 set_goals。
		MaxTurns:     8,
		NonStreaming: nonStreaming, // 该 profile 选非流式时走 Provider.Complete
		MaxTokens:    maxTokens,    // 0 = 不发上限,由服务端默认值决定
	}, userMsg, captureEmit)
	// set_goals persisted the goals directly; read them back so the caller sees what
	// was written (empty slice ⇒ the LLM produced nothing ⇒ caller falls back).
	if ts == nil {
		return nil
	}
	nodes, _ := ts.ListByKind(db.KindGoal, 10000)
	var out []GoalSpec
	for _, n := range nodes {
		var p struct {
			Text      string `json:"text"`
			VulnClass string `json:"vulnclass"`
		}
		_ = json.Unmarshal(n.Payload, &p)
		if strings.TrimSpace(p.Text) != "" {
			out = append(out, GoalSpec{Text: p.Text, VulnClass: p.VulnClass})
		}
	}
	return out
}
