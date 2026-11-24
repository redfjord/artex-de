package agent

import (
	"context"

	"github.com/Autumn-27/artex/db"
)

// FindingRecorder is injected by the host; agents never synthesize or copy
// evidence bodies themselves. Its implementation owns the atomic write.
type FindingRecorder interface {
	Record(context.Context, db.RecordFindingInput, []db.TrafficRef) (*db.RecordedFinding, error)
}

// Tool-use guidance is appended without replacing the user's editable prompt.
// It does not require capture or claim that unavailable traffic tools exist.
const findingTrafficGuidance = "\n\n**Schwachstellen-Datenverkehrsnachweis (optional)**: Beim Aufruf von report_finding zur Meldung einer Schwachstelle können, sofern bereits geprüfte und bestätigte HTTP-Anfragen/Antworten vorliegen, die die Schlussfolgerung stützen, echte IDs über traffic_refs in Reproduktionsreihenfolge gebunden werden; Domains und Zeitstempel dienen nur als Kandidatenfilter, keine Assoziation wird vorausgesetzt. Bei nicht-HTTP-Schwachstellen (z. B. TCP), nicht erfassten Daten oder fehlender exakter Übereinstimmung weglassen oder [] übergeben; in evidence Befehlsausgaben, Logs und andere verifizierbare Nachweise beibehalten, Grund für Nicht-Bindung angeben. IDs nicht raten, nicht nur zum Nachfassen wiederholte Probenahme durchführen."

func (t *ToolSet) SetFindingRecorder(r FindingRecorder)   { t.findingRecorder = r }
func (w *Worker) SetFindingRecorder(r FindingRecorder)    { w.findingRecorder = r }
func (p *Planner) SetFindingRecorder(r FindingRecorder)   { p.findingRecorder = r }
func (m *MainAgent) SetFindingRecorder(r FindingRecorder) { m.findingRecorder = r }
