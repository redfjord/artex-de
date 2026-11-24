package agent

import (
	"strings"

	"github.com/Autumn-27/artex/db"
)

// constraintBlock renders this task's operation constraints (task_constraints) as a
// high-priority block appended to the planner/worker system prompt. allow/deny are
// grouped; empty string when there are no constraints (or ts is nil). The framing
// deliberately puts these ABOVE the exploration/expansion heuristics so a declared
// boundary wins the tug-of-war against "chase another entry surface".
func constraintBlock(ts *db.ExplorationStore) string {
	if ts == nil {
		return ""
	}
	rows, err := ts.ListConstraints()
	if err != nil || len(rows) == 0 {
		return ""
	}
	var allow, deny []string
	for _, c := range rows {
		text := strings.TrimSpace(c.Text)
		if text == "" {
			continue
		}
		if c.Kind == "allow" {
			allow = append(allow, "- "+text)
		} else {
			deny = append(deny, "- "+text)
		}
	}
	if len(allow) == 0 && len(deny) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n【Operationsbeschränkungen (höchste Priorität, übersteuert alle unten stehenden Erkundungs-/Expansions-Heuristiken; vor Erzeugung jeder Absicht und vor Ausführung jeder Aktion muss zuerst geprüft werden, ob ein Verstoß vorliegt; bei Verstoß ist die Ausführung unzulässig)】:")
	if len(allow) > 0 {
		b.WriteString("\nErlaubte Operationen:\n")
		b.WriteString(strings.Join(allow, "\n"))
	}
	if len(deny) > 0 {
		b.WriteString("\nVerbotene Operationen:\n")
		b.WriteString(strings.Join(deny, "\n"))
	}
	b.WriteString("\n(Neue Ziele/neue Ports/neue Hosts außerhalb der erkannten Beschränkungen bedeuten keine Autorisierung: Es sei denn, sie fallen in den oben genannten erlaubten Bereich, andernfalls als Out-of-Scope-Fakt erfassen und überspringen, keine Absichten oder Aktionen dafür ableiten.)")
	return b.String()
}
