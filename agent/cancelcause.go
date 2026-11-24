package agent

import (
	"context"
	"errors"
	"fmt"
)

// AbortCause names why an agent run's context was cancelled. Every cancellation
// site should attach one so the activity trace can report the real initiator.
type AbortCause struct {
	Code  string
	Short string
	Text  string
}

func (c *AbortCause) Error() string { return c.Text }

func cause(code, short, text string) *AbortCause {
	return &AbortCause{Code: code, Short: short, Text: text}
}

// Causef builds a cause that includes runtime-specific detail.
func Causef(code, short, format string, args ...any) *AbortCause {
	return &AbortCause{Code: code, Short: short, Text: fmt.Sprintf(format, args...)}
}

var (
	// Task-level execution context.
	AbortPausedByUser = cause("paused_by_user", "Benutzer hat die Aufgabe pausiert",
		"Benutzer hat die Aufgabe über die Aufgabensteuerungsschnittstelle (POST /api/tasks/{id}/control, action=pause) pausiert. Dieser Planner/Worker-Lauf wurde aktiv abgebrochen; laufende Intentionen werden an frontier(open) zurückgegeben und bei Wiederaufnahme der Aufgabe neu beansprucht und von vorne ausgeführt")
	AbortPausedByOrchestrator = cause("paused_by_orchestrator", "Orchestrator hat die Aufgabe pausiert",
		"Orchestrator-Agent hat das pause_task-Tool aufgerufen, um diese Aufgabe zu pausieren. Dieser Planner/Worker-Lauf wurde aktiv abgebrochen; laufende Intentionen werden an frontier(open) zurückgegeben und bei Wiederaufnahme neu ausgeführt")
	AbortTaskDeleted = cause("task_deleted", "Aufgabe wurde gelöscht",
		"Aufgabe wird gelöscht (DELETE /api/tasks/{id}), die Löschbarriere hat den laufenden Planner, Worker und Haupt-Agent dieser Aufgabe abgebrochen; das Ergebnis dieses Laufs wird nicht mehr verwendet")
	AbortPausedOnReload = cause("paused_on_reload", "Pausestatus nach Neustart übernommen",
		"Beim Backend-Start wurde der Aufgabenpausestatus basierend auf dem in der Datenbank persistierten Status wiederhergestellt. Dieser Lauf wurde abgebrochen; normalerweise gibt es in der Wiederherstellungsphase keine laufenden Agenten")
	AbortGoalMet = cause("goal_met", "Planer: Aufgabenziel erreicht",
		"Planer hat festgelegt, dass das Aufgabenziel erreicht wurde und die Aufgabe auf done gesetzt, anschließend noch laufende Worker abgebrochen; diese Intentionen werden als stopped markiert, nicht als fehlgeschlagen")
	AbortSettleDrainTimeout = cause("settle_drain_timeout", "Timeout-Abwicklung: Wartezeit abgelaufen",
		"Nach Erreichen des Timeouts wurde auf das ordnungsgemäße Beenden der laufenden Worker gewartet, aber die 90-sekündige Drain-Karenzzeit reichte nicht aus, daher wird ein harter Abbruch ausgeführt; Intentionen werden als exhausted markiert, in der Abwicklungsphase bereits geschriebene Fakten und Assets werden beibehalten")

	// Per-work context.
	AbortKilledByPlanner = cause("killed_by_planner", "Planer hat diese Intention beendet",
		"Planer hat kill_work aufgerufen, um diese Intention aktiv zu beenden, meist weil die Richtung abwich oder kein weiterer Wert bestand; Intention wird als stopped markiert, nicht automatisch neu beansprucht")
	AbortWorkPausedByUser = cause("work_paused_by_user", "Benutzer hat die Intention pausiert",
		"Benutzer hat den laufenden Worker pausiert. Dieser Aufruf wurde abgebrochen, Intention wird zu paused; bereits registrierte Intentionen, Fakten, Schwachstellen und Aktivitätsprotokolle werden alle beibehalten, bei Wiederaufnahme von vorne neu ausgeführt")
	AbortWorkCancelledByUser = cause("work_cancelled_by_user", "Benutzer hat die Intention gelöscht",
		"Benutzer hat den laufenden Worker gelöscht. Dieser Aufruf wurde abgebrochen; nach Verlassen des Schreibbereichs durch den Worker verarbeitet der Server die Intention gemäß dem vom Benutzer gewählten Löschmodus – Weichlöschung markiert nur als gelöscht und behält alle Ausgaben, Hartlöschung entfernt kaskadierend diese Intention und die ausschließlich von ihr getragenen nachgelagerten Knoten")
	AbortWorkFinished = cause("work_finished", "Worker normal beendet",
		"Worker wurde normal beendet, Engine gibt seine Context-Ressourcen in detachWork frei. Dies ist keine Laufunterbrechung; falls es in Abbruchnachrichten erscheint, deutet dies auf einen Race zwischen Abbruch und Abwicklungsereignis hin")
	AbortPausedRaceGuard = cause("paused_race_guard", "Aufgabe pausiert: kein neuer Lauf",
		"Wenn sich die Aufgabe im Pausestatus befindet, verweigert die Engine die Ausgabe neuer Ausführungs-Contexts, um zu verhindern, dass Worker durch Race zwischen Claim und Pause weiter starten; bereits beanspruchte Intentionen werden an frontier zurückgegeben")

	// Main Agent and standalone conversation contexts.
	AbortChatStoppedByUser = cause("chat_stopped_by_user", "Benutzer hat die Runde gestoppt",
		"Benutzer hat auf Stopp geklickt, laufenden Haupt-Agent oder Sitzungs-Agent dieser Runde aktiv abgebrochen. Bereits erzeugte Aktivitätsprotokolle werden beibehalten, nächste Nachricht kann weitergesendet werden")
	AbortChatPausedWithTask = cause("chat_paused_with_task", "Aufgabe pausiert, Gespräch abgebrochen",
		"Wenn Benutzer Aufgabe pausiert, wird auch laufendes Haupt-Agent-Gespräch synchron abgebrochen. Bereits erzeugte Aktivitätsprotokolle werden beibehalten; nach Wiederaufnahme der Aufgabe wird diese Gesprächsrunde nicht automatisch wiederholt")
	AbortChatTurnFinished = cause("chat_turn_finished", "Gesprächsrunde normal beendet",
		"Diese Gesprächsrunde wurde normal beendet, Server gibt die Context-Ressourcen dieser Runde frei. Dies ist keine Laufunterbrechung; falls es in Abbruchnachrichten erscheint, deutet dies auf einen Race zwischen Abbruch und Abwicklungsereignis hin")

	// Process-level and per-run hard backstop.
	AbortShutdown = cause("shutdown", "Backend-Prozess wird heruntergefahren",
		"Backend-Prozess hat SIGINT oder SIGTERM empfangen, wird neu gestartet, aktualisiert oder heruntergefahren. Alle laufenden Agenten werden abgebrochen; nach Neustart werden verbleibende running-Intentionen auf open zurückgesetzt und neu ausgeführt")
	AbortRunHardTimeout = cause("run_hard_timeout", "Hartes Lauf-Timeout ausgelöst",
		"Einzelner Lauf hat Soft-Wanduhr-Budget und zusätzliche Karenz überschritten, was bedeutet, dass Modellanfrage oder ein Tool lange nicht zurückgegeben hat, sodass normale Rundenabwicklung nicht ausgeführt werden konnte. Bitte prüfen Sie schwerpunktmäßig den letzten vor dem Abbruch nicht zurückgekehrten Tool-Aufruf")
)

// AbortReason resolves the named cause attached to a cancelled run context.
func AbortReason(ctx context.Context) (code, short, text string, ok bool) {
	c := context.Cause(ctx)
	if c == nil {
		return "", "", "", false
	}
	var ac *AbortCause
	if errors.As(c, &ac) {
		return ac.Code, ac.Short, ac.Text, true
	}
	switch {
	case errors.Is(c, context.DeadlineExceeded):
		return "deadline_exceeded", "Upstream-Context hat Deadline erreicht",
			"Upstream-Context hat Deadline erreicht, aber Setzer hat keine benannte Ursache über WithTimeoutCause angehängt: " + c.Error(), true
	case errors.Is(c, context.Canceled):
		return "canceled_no_cause", "Abbruchender hat keine benannte Ursache angehängt",
			"Upstream-Context wurde abgebrochen, aber Abbruchender hat keine benannte Ursache über context.WithCancelCause angehängt; bitte in agent/cancelcause.go Ursache registrieren und an diesen Abbruchpunkt anbinden", true
	default:
		return "other", firstLine(c.Error(), 80), c.Error(), true
	}
}
