package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Autumn-27/norma/harness"
)

// runTrace retains the latest tool call so an interrupted run can identify the
// operation that was still in flight.
type runTrace struct {
	startedAt time.Time
	id        string
	name      string
	input     string
	at        time.Time
	pending   bool
}

func (t *runTrace) start(id, name, input string) {
	t.id, t.name, t.input, t.at, t.pending = id, name, input, time.Now(), true
}

func (t *runTrace) done(id string) {
	if id == t.id {
		t.pending = false
	}
}

var reasonHint = map[harness.TerminalReason]string{
	harness.ReasonCompleted:         "Das Modell hat die Runde normal beendet, aber keine Textzusammenfassung hinterlassen; Fakten und Assets basieren auf den Tool-Aufruf-Aufzeichnungen dieser Runde",
	harness.ReasonMaxTurns:          "Schrittzahl-Obergrenze erreicht (MaxTurns): SDK hat Abschluss durchgeführt und Fakten sowie Assets zurückgeschrieben, Intent wird als exhausted markiert, damit der Planer die Richtung wechseln kann, statt als Fehlschlag zu werten",
	harness.ReasonTimeout:           "Wanduhr-Budget für einzelnen Lauf erreicht (MaxDuration): Zum Zeitpunkt wird laufendes Tool unterbrochen und vor Ort Abschluss durchgeführt, erkannte Fakten und Assets werden zurückgeschrieben, Intent wird als exhausted markiert",
	harness.ReasonModelError:        "Modell- oder API-Aufruf fehlgeschlagen (Netzwerk, Auth, Rate-Limit, Anbieter 5xx etc.), nach Ausschöpfen der Wiederholungsversuche wird Intent als blocked markiert — Transportschicht-Fehler führt dazu, dass dieser Intent im Grunde nicht wirklich erkundet wurde; nach Prüfung der Ausführung (get_worker_trace) entscheiden über erneute Entsendung oder andere Methode",
	harness.ReasonBlockingLimit:     "Kontextlänge hat harte Obergrenze erreicht, Anfrage wurde vor Absendung abgefangen; Intent-Granularität verringern oder Tool-Rückgaben komprimieren",
	harness.ReasonPromptTooLong:     "Prompt zu lang und Kontext-Komprimierungs-Wiederholungen ausgeschöpft, kann nicht fortfahren",
	harness.ReasonImageError:        "Aktuelles Modell unterstützt multimodale Inhalte dieser Runde nicht; bitte auf visuelles Modell wechseln oder Tool-Bildrückgaben vermeiden",
	harness.ReasonStopHookPrevented: "Stop-Hook hat Rundenende verhindert, konnte anschließend nicht fortfahren; bitte prüfen, ob Task-Guard-Regeln zu streng sind",
	harness.ReasonHookStopped:       "Tool oder Hook hat Weiterausführung aktiv gestoppt, z. B. Ziel außerhalb des Bereichs oder deaktivierter Befehl; bitte Abfang-Erklärung im letzten tool_result prüfen",
	harness.ReasonAbortedStreaming:  "Lauf während Modell-Ausgabe-Streaming-Phase abgebrochen",
	harness.ReasonAbortedTools:      "Lauf während Tool-Ausführungsphase abgebrochen",
}

// terminalText renders a terminal event with no final text into a compact summary
// and a Markdown detail block.
func terminalText(ctx context.Context, term *harness.Terminal, tr *runTrace) (string, string) {
	reason := term.Reason
	aborted := reason == harness.ReasonAbortedStreaming || reason == harness.ReasonAbortedTools
	// Prompt may return ctx.Err directly without a terminal event. Preserve the
	// cancellation cause instead of falling back to an empty/unknown terminal reason.
	if reason == "" && ctx.Err() != nil {
		aborted = true
	}

	var sum string
	if aborted {
		_, short, _, ok := AbortReason(ctx)
		if !ok {
			short = "Abbruchgrund konnte nicht ermittelt werden"
		}
		stage := "während der Ausführung"
		switch reason {
		case harness.ReasonAbortedStreaming:
			stage = "Modell-Ausgabephase"
		case harness.ReasonAbortedTools:
			stage = "Tool-Ausführungsphase"
		}
		sum = "(Lauf unterbrochen: " + short + "; gestoppt in " + stage + progressSuffix(term, tr) + ", unvollständig)"
	} else if reason == harness.ReasonMaxTurns || reason == harness.ReasonTimeout {
		sum = "(Laufbudget-Obergrenze erreicht (" + string(reason) + "), Abschluss durchgeführt und Fakten zurückgeschrieben" + progressSuffix(term, tr) + "; diesmal keine Textzusammenfassung)"
	} else {
		hint := terminalReasonHint(reason)
		sum = "(Keine Textzusammenfassung, Endzustand " + terminalReasonLabel(reason) + ": " + firstLine(hint, 80) + ")"
	}

	var b strings.Builder
	b.WriteString(sum)
	b.WriteString("\n\n")
	displayReason := terminalReasonLabel(reason)
	fmt.Fprintf(&b, "- **Endzustand**: `%s` - %s\n", displayReason, terminalReasonHint(reason))
	if aborted {
		code, _, why, ok := AbortReason(ctx)
		if ok {
			fmt.Fprintf(&b, "- **Unterbrechungsgrund** (`%s`): %s\n", code, why)
		} else {
			b.WriteString("- **Unterbrechungsgrund**: Konnte nicht ermittelt werden; Abbrecher hat möglicherweise keinen benannten Grund über context.WithCancelCause angehängt\n")
		}
	}
	if term.Err != nil {
		fmt.Fprintf(&b, "- **Unterliegender Fehler**: `%v`\n", term.Err)
	}
	if aborted && strings.TrimSpace(term.Text) != "" {
		b.WriteString("- **Vor Abbruch generierte Teilausgabe**:\n\n")
		b.WriteString(term.Text)
		b.WriteString("\n\n")
	}
	if term.Turns > 0 {
		fmt.Fprintf(&b, "- **Ausgeführt**: %d Modell-Runden\n", term.Turns)
	}
	if !tr.startedAt.IsZero() {
		fmt.Fprintf(&b, "- **Laufzeit diesmal**: %s\n", roundDur(time.Since(tr.startedAt)))
	}
	if u := term.Usage; u.InputTokens+u.OutputTokens+u.CacheReadTokens+u.CacheWriteTokens > 0 {
		fmt.Fprintf(&b, "- **Kumulierte Token**: Eingabe %d / Ausgabe %d / Cache-Lesen %d / Cache-Schreiben %d\n",
			u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens)
	}
	if tr.name == "" {
		b.WriteString("- **Tool-Aufrufe**: Dieser Lauf endete ohne Tool-Aufrufe\n")
	} else if tr.pending {
		fmt.Fprintf(&b, "- **Bei Abbruch ausgeführtes Tool**: `%s` (läuft seit %s, **kein Ergebnis zurückgegeben**)\n\n  ```json\n  %s\n  ```\n",
			tr.name, roundDur(time.Since(tr.at)), firstLine(tr.input, 300))
	} else {
		fmt.Fprintf(&b, "- **Letztes Tool vor Abbruch**: `%s` (normal zurückgegeben)\n", tr.name)
	}
	return sum, b.String()
}

func terminalReasonLabel(reason harness.TerminalReason) string {
	if reason == "" {
		return "context_canceled"
	}
	return string(reason)
}

func terminalReasonHint(reason harness.TerminalReason) string {
	if hint := reasonHint[reason]; hint != "" {
		return hint
	}
	if reason == "" {
		return "Lauf-Context wurde abgebrochen, aber untere Schicht hat kein Terminal-Ereignis erzeugt"
	}
	return "Unbekannter Endzustand; harness hat möglicherweise TerminalReason hinzugefügt, bitte reasonHint ergänzen"
}

func progressSuffix(term *harness.Terminal, tr *runTrace) string {
	var parts []string
	if term.Turns > 0 {
		parts = append(parts, fmt.Sprintf("%d Runden", term.Turns))
	}
	if !tr.startedAt.IsZero() {
		parts = append(parts, roundDur(time.Since(tr.startedAt)))
	}
	if len(parts) == 0 {
		return ""
	}
	return ", gelaufen " + strings.Join(parts, " / ")
}

func roundDur(d time.Duration) string {
	switch {
	case d < time.Minute:
		return d.Round(100 * time.Millisecond).String()
	case d < time.Hour:
		return d.Round(time.Second).String()
	default:
		return d.Round(time.Minute).String()
	}
}
