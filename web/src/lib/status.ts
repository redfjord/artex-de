// Centralised status → color/label semantics, reused across the whole app.
// Spec §8.3: intent / coverage / task / severity each have a consistent color set.

export type Tone = "neutral" | "blue" | "green" | "amber" | "red" | "rose" | "violet" | "slate";

export const toneClasses: Record<Tone, string> = {
  neutral: "bg-muted text-muted-foreground border-transparent",
  blue: "bg-blue-500/15 text-blue-600 dark:text-blue-400 border-blue-500/20",
  green: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/20",
  amber: "bg-amber-500/15 text-amber-600 dark:text-amber-400 border-amber-500/20",
  red: "bg-red-500/15 text-red-600 dark:text-red-400 border-red-500/20",
  // rose is used for "critical" — solid high emphasis, visually stronger than "high"'s soft red outline.
  rose: "bg-rose-600 text-white border-rose-600 dark:bg-rose-600 dark:text-white",
  violet: "bg-violet-500/15 text-violet-600 dark:text-violet-400 border-violet-500/20",
  slate: "bg-slate-500/15 text-slate-600 dark:text-slate-400 border-slate-500/20",
};

export const toneDot: Record<Tone, string> = {
  neutral: "bg-muted-foreground",
  blue: "bg-blue-500",
  green: "bg-emerald-500",
  amber: "bg-amber-500",
  red: "bg-red-500",
  rose: "bg-white",
  violet: "bg-violet-500",
  slate: "bg-slate-500",
};

interface StatusMeta {
  label: string;
  tone: Tone;
}

const intent: Record<string, StatusMeta> = {
  open: { label: "Ausstehend", tone: "slate" },
  running: { label: "Läuft", tone: "blue" },
  paused: { label: "Pausiert", tone: "amber" },
  done: { label: "Fertig", tone: "green" },
  // blocked = model/API/network failures exhausted retries; this intent was largely never actually explored (not a goal intercept).
  blocked: { label: "Fehler", tone: "red" },
  // exhausted = cut off midway after hitting the step/time budget, with only partial results written back (not an exhausted direction).
  exhausted: { label: "Budget erschöpft", tone: "violet" },
  // stopped = legacy soft-delete state (retained for historical data).
  stopped: { label: "Gestoppt", tone: "slate" },
  // deleted = the user soft-deleted this intent (nodes and lineage retained; see the delete_reason field).
  deleted: { label: "Gelöscht", tone: "slate" },
};

const task: Record<string, StatusMeta> = {
  created: { label: "Erstellt", tone: "slate" },
  queued: { label: "In Warteschlange", tone: "amber" },
  running: { label: "Läuft", tone: "blue" },
  paused: { label: "Pausiert", tone: "amber" },
  done: { label: "Fertig", tone: "green" },
  failed: { label: "Fehlgeschlagen", tone: "red" },
  timeout: { label: "Zeitüberschreitung", tone: "amber" },
};

const severity: Record<string, StatusMeta> = {
  critical: { label: "Kritisch", tone: "rose" },
  high: { label: "Hoch", tone: "red" },
  medium: { label: "Mittel", tone: "amber" },
  low: { label: "Niedrig", tone: "slate" },
};

const finding: Record<string, StatusMeta> = {
  pending: { label: "Ausstehend", tone: "amber" },
  in_progress: { label: "In Bearbeitung", tone: "blue" },
  confirmed: { label: "Bestätigt", tone: "red" },
  resolved: { label: "Behoben", tone: "green" },
  fixed: { label: "Behoben", tone: "green" },
  false_positive: { label: "Falsch-Positiv", tone: "slate" },
  ignored: { label: "Ignoriert", tone: "neutral" },
  duplicate: { label: "Duplikat", tone: "neutral" },
  risk_accepted: { label: "Risiko akzeptiert", tone: "violet" },
};

const engine: Record<string, StatusMeta> = {
  exploring: { label: "Erkundung", tone: "blue" },
  paused: { label: "Pausiert", tone: "amber" },
  stalled: { label: "Ins Stocken geraten", tone: "red" },
  idle: { label: "Inaktiv", tone: "neutral" },
};

const goal: Record<string, StatusMeta> = {
  open: { label: "In Bearbeitung", tone: "blue" },
  met: { label: "Erreicht", tone: "green" },
  abandoned: { label: "Aufgegeben", tone: "slate" },
};

const audit: Record<string, StatusMeta> = {
  allow: { label: "Erlauben", tone: "green" },
  block: { label: "Blockieren", tone: "red" },
};

const node: Record<string, StatusMeta> = {
  observed: { label: "Beobachtet", tone: "slate" },
  confirmed: { label: "Bestätigt", tone: "green" },
  tombstoned: { label: "Tombstoned", tone: "neutral" },
};

// Delivery status. sending uses blue rather than amber: it doesn't mean "something is wrong",
// but "claimed and being sent", which must be distinguishable from pending's waiting semantics.
const delivery: Record<string, StatusMeta> = {
  pending: { label: "Ausstehend", tone: "amber" },
  sending: { label: "Wird gesendet", tone: "blue" },
  sent: { label: "Zugestellt", tone: "green" },
  failed: { label: "Fehlgeschlagen", tone: "red" },
  skipped: { label: "Übersprungen", tone: "neutral" },
};

const maps = {
  intent,
  task,
  severity,
  finding,
  engine,
  goal,
  audit,
  node,
  delivery,
} as const;

export type StatusDomain = keyof typeof maps;

export function statusMeta(domain: StatusDomain, key: string): StatusMeta {
  return maps[domain][key] ?? { label: key, tone: "neutral" };
}
