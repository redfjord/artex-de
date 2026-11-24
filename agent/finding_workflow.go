package agent

import (
	"encoding/json"
	"fmt"

	"github.com/Autumn-27/artex/db"
	actool "github.com/Autumn-27/norma/tool"
)

const findingIDGuidance = "\n\n**Schwachstellen-Nummerierungskonvention**: finding_id ist die unabhängige Schwachstellen-Aufzeichnungs-ID; finding_node_id ist die Erkundungsknoten-ID. Die id von list_findings / list_task_findings / node_detail / get_task_node_detail bleibt die Erkundungsknoten-ID, die unabhängige Nummer sollte aus dem finding_id derselben Rückgabe gelesen werden. get_finding_traffic / bind_finding_traffic verwenden die unabhängige finding_id. Der finding_id-Parameter des alten update_finding_report wird weiterhin finding_node_id übergeben. Verwenden Sie nicht die Zahl der ersten Zeile von report_finding für Beweiswerkzeuge, und raten Sie nicht andere Zahlen bei Nummerierungsfehlern."

// The server supplies the persisted setting. A missing setting/host is off.
// Consulted at assembly and again on writes so an already-running session
// cannot keep binding after the user switches the feature off.
var FindingTrafficBindingEnabled func() bool

func findingTrafficBindingEnabled() bool {
	return FindingTrafficBindingEnabled != nil && FindingTrafficBindingEnabled()
}

// Applied after ToolResolve: user descriptions and prompts remain intact, while
// all actual reporters (including Planner and custom chat agents) see the same
// API contract. Disabled/unbound tools are never reintroduced here.
func findingWorkflowTools(agentKey string, tools []actool.CoreTool) ([]actool.CoreTool, string) {
	if !findingTrafficBindingEnabled() {
		out := make([]actool.CoreTool, 0, len(tools))
		for _, tool := range tools {
			if tool.Name() == "bind_finding_traffic" {
				continue
			}
			if agentKey == "reporter" && (tool.Name() == "traffic_search" || tool.Name() == "traffic_get" || tool.Name() == "traffic_blob") {
				continue
			}
			switch tool.Name() {
			case "report_finding", "add_hint", "add_task_hint":
				// Work on a copy: toggling back on must restore the original schema.
				raw, _ := json.Marshal(tool.InputSchema())
				var schema map[string]any
				if json.Unmarshal(raw, &schema) == nil {
					stripTrafficParameters(schema)
					tool = DecorateTool(tool, tool.Description(), schema)
				}
			}
			out = append(out, tool)
		}
		return out, ""
	}
	out := append([]actool.CoreTool(nil), tools...)
	has := map[string]bool{}
	for i, tool := range out {
		has[tool.Name()] = true
		note := ""
		switch tool.Name() {
		case "report_finding":
			note = "\nStandardmäßig wird vom Berichts-Agent vor dem Verfassen des Berichts der Datenverkehr geprüft und gebunden. Der Melder behält in evidence Verifizierungsbefehle, Schlüsselausgaben, vorhandene echte Datenverkehr-IDs und deren Zweck bei, damit der Berichts-Agent diese anhand der Ausführungsaufzeichnungen prüfen kann; keine zusätzliche Paketabfrage für die Bindung erforderlich. Kompatibel mit expliziter sofortiger Bindung: traffic_refs oder evidence_hint_id können verifizierte Referenzen übermitteln, letztere liest die strukturierten Referenzen des für diese Aufgabe angegebenen Hinweises; ist einer ungültig, schlägt die gesamte Meldung fehl. TCP/kein Paket benötigen diese optionalen Parameter nicht. Die zurückgegebenen finding_id und finding_node_id stehen jeweils für unabhängige Aufzeichnung und Erkundungsknoten."
		case "add_hint", "add_task_hint":
			note = "\nBei der Übergabe bestätigter Schwachstellen werden in traffic_refs des entsprechenden Hinweises die IDs, Zwecke, Erklärungen und die Reihenfolge der verifizierten Datenströme beibehalten (einzelne oben, mehrere in den entsprechenden hints-Elementen), und in text wird beschrieben, welche konkrete Schwachstelle sie beweisen. Der Aufrufer darf nicht nur Text übergeben und dabei vorhandene Datenverkehrsreferenzen verwerfen. Nicht verifizierte Kandidaten dürfen nicht als Beweis übergeben werden."
		case "get_finding_traffic", "bind_finding_traffic", "list_findings", "list_task_findings", "node_detail", "get_task_node_detail", "update_finding_report":
			note = findingIDGuidance
		}
		if note != "" {
			out[i] = DecorateTool(tool, tool.Description()+note, tool.InputSchema())
		}
	}
	guidance := ""
	if has["report_finding"] || has["add_task_hint"] || has["add_hint"] {
		guidance = "\n\n**Datenverkehrs-Beweisübergabe (optional)**: Die automatische Bindung wird standardmäßig vom Berichts-Agent nach der Schwachstellen-Speicherung und vor dem Verfassen des Berichts abgeschlossen. Der Melder sollte in evidence Verifizierungsbefehle, Schlüsselausgaben, vorhandene echte Datenverkehr-IDs und deren Zweck beibehalten, die Aufgabe sollte intent_id enthalten, damit der Berichts-Agent zurückverfolgen kann; keine zusätzliche Paketabfrage für die Bindung erforderlich. Wenn Auto/Planner stellvertretend meldet, die Referenzen des Ausführers nicht verwerfen. add_hint / add_task_hint können traffic_refs zur Übergabe nutzen; explizite sofortige Bindung bleibt kompatibel mit report_findings traffic_refs / evidence_hint_id. Bei TCP oder fehlendem Paket normal registrieren, IDs nicht erraten, auch nicht nur zum Nachholen von Paketen wiederholt sondieren."
		if has["add_task_hint"] && !has["add_hint"] {
			guidance += "\nWenn der Plattform-Dialog keinen Aufgabenkontext hat, report_finding nicht direkt aufrufen; stattdessen über add_task_hint an die entsprechende bestehende Aufgabe übergeben, vom Aufgaben-Agent registrieren lassen und mit list_task_findings das Ergebnis prüfen."
		}
		if has["prove_goal"] || has["goal_met"] {
			guidance += "\nBevor das Ziel als abgeschlossen gewertet wird, zuerst die vorliegende Beweisaufnahme/-übergabe dieses Durchlaufs abschließen. Die Aufgabe nicht beenden und Worker nicht abbrechen, nur weil die textuelle Schwachstelle bereits registriert ist, solange die Beweisübergabe noch nicht abgeschlossen ist; bei fehlendem Paket kein Warten oder erzwungenes Abfangen erforderlich."
		}
	}
	if has["update_finding_report"] && has["bind_finding_traffic"] && has["get_finding_traffic"] {
		guidance += "\n\n**Automatische Datenverkehrs-Zuordnung vor dem Bericht (aktiviert)**: Sie sind verantwortlich für die Prüfung und Bindung des Datenverkehrs für die in diesem Durchlauf ausgelöste Schwachstelle, bevor Sie den Bericht verfassen. Zunächst aus dem report_finding-Rückgabe-JSON oder get_task_node_detail / list_task_findings die eindeutigen finding_id und finding_node_id ermitteln. Schwachstellendetails, Ausführungsaufzeichnungen der zugehörigen Absicht und vorhandene Beweisliste lesen, dabei die vom Melder übergebenen echten IDs vorrangig verwenden. Falls die diesseitige Verifizierung HTTP ist und Datenverkehrs-Werkzeuge verfügbar sind, mit traffic_search Kandidaten filtern, dann mit traffic_get jede einzeln prüfen, ob Anfrage/Antwort die Schwachstelle tatsächlich stützen; Domain und Zeit dienen nur der Filterung, nicht dem Eigentumsnachweis. Bestätigte Beweise in Reproduktionsreihenfolge mit bind_finding_traffic(finding_id, traffic_refs) verknüpfen, baseline / proof / verification / supporting wählen und Zweck angeben. Nur die diesseitige Schwachstelle bearbeiten, keine erneute Schwachstellenerstellung oder erneutes Zielen. Nach erfolgreicher Bindung get_finding_traffic erneut aufrufen für die neueste version, benötigte Inhalte lesen, dann die tatsächlich gelesene version als evidence_version an update_finding_report übergeben (dessen finding_id-Parameter weiterhin finding_node_id verwendet). Bereits bestehende Bindungen nicht erneut hinzufügen. Bei TCP, nicht erfasst, Werkzeuge nicht verfügbar oder keine exakte Übereinstimmung: automatische Bindung überspringen, Bericht basierend auf Text/Befehlsbeweisen normal verfassen und Grund angeben, nicht zum Vervollständigen des Datenverkehrs raten. Bei Bindungsfehler keinen Erfolg behaupten; vorhandene Beweise behalten und im Bericht Grund der nicht erfolgten Bindung angeben."
	}
	if guidance != "" || has["get_finding_traffic"] || has["update_finding_report"] {
		guidance += findingIDGuidance
	}
	return out, guidance
}

func stripTrafficParameters(schema map[string]any) {
	props, _ := schema["properties"].(map[string]any)
	delete(props, "traffic_refs")
	delete(props, "evidence_hint_id")
	if required, ok := schema["required"].([]any); ok {
		kept := required[:0]
		for _, key := range required {
			if key != "traffic_refs" && key != "evidence_hint_id" {
				kept = append(kept, key)
			}
		}
		schema["required"] = kept
	}
	if hints, ok := props["hints"].(map[string]any); ok {
		if items, ok := hints["items"].(map[string]any); ok {
			stripTrafficParameters(items)
		}
	}
}

// HintTrafficSchema is shared by the task-local and cross-task hint tools.
func HintTrafficSchema() map[string]any {
	return map[string]any{"type": "array", "description": "Optional: Verifizierte Datenverkehrsreferenzen, die der konkreten Schwachstelle in diesem Hinweis entsprechen, Reihenfolge beibehalten; nach Übergabe kann report_finding evidence_hint_id übergeben, um diese Referenzen mitzuführen.", "items": obj(map[string]any{"traffic_id": str("Echte Datenverkehrs-ID"), "role": str("baseline / proof / verification / supporting"), "note": str("Welche Schlussfolgerung dieser Datenverkehr stützt")}, "traffic_id")}
}

func (t *ToolSet) findingRefsFromHint(hintID int64, explicit []db.TrafficRef) ([]db.TrafficRef, error) {
	if hintID <= 0 {
		return db.NormalizeTrafficRefs(explicit)
	}
	n, err := t.ts.GetNode(hintID) // local store only: inherited hints cannot supply evidence
	if err != nil {
		return nil, err
	}
	if n == nil || n.Kind != db.KindHint {
		return nil, fmt.Errorf("evidence_hint_id=%d muss ein Hinweisknoten dieser Aufgabe sein (übernommene Hinweise können nicht direkt für die Bindung verwendet werden)", hintID)
	}
	var payload struct {
		Refs []db.TrafficRef `json:"traffic_refs"`
	}
	if err := json.Unmarshal(n.Payload, &payload); err != nil {
		return nil, err
	}
	return db.NormalizeTrafficRefs(append(append([]db.TrafficRef{}, explicit...), payload.Refs...))
}
