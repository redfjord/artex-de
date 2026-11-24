"use client";

// Shared pieces for LLM retry settings: the "attempts + interval" of each of the five retry layers.
//
// The five layers, from innermost to outermost: connect (SDK) → empty response (SDK) → same-provider safe window → polling breaker → intent rerun.
// The first three follow the endpoint, so each model profile can override the global defaults; the last two are process-wide and only have a global copy.
//
// All inputs follow the same "empty = not configured" semantics, matching the backend db.RetryRule:
//   attempts  empty/0 = use built-in default | -1 = disable this retry layer | >0 = use this count
//   interval  empty/0 = use this layer's original exponential backoff | >0 = use this fixed millisecond interval

import * as React from "react";

import { Loader2Icon, SaveIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import type { LLMRetryOverride, LLMRetryPolicy, LLMRetryRule } from "@/lib/types";

export const ZERO_RULE: LLMRetryRule = { attempts: 0, interval_ms: 0 };
export const ZERO_OVERRIDE: LLMRetryOverride = {
  connect: ZERO_RULE,
  empty: ZERO_RULE,
  stream: ZERO_RULE,
};
const ZERO_POLICY: LLMRetryPolicy = {
  ...ZERO_OVERRIDE,
  breaker: ZERO_RULE,
  intent: ZERO_RULE,
};

type LayerMeta = {
  title: string;
  /** Where this retry layer lives and who runs it */
  where: string;
  /** What kind of errors reach this layer — down to the status codes, so nobody has to guess */
  trigger: string;
  /** Errors that look similar but do [not] go through this layer, so a no-op setting is not mistaken for a bug */
  skips?: string;
  desc: string;
  attemptsLabel: string;
  /** Default when attempts is empty, used for the placeholder */
  defAttempts: number;
  /** Default policy when interval is empty, used for the placeholder */
  defInterval: string;
  /** Meaning of entering -1 for attempts */
  offHint: string;
};

export const RETRY_LAYERS = {
  connect: {
    title: "Verbindungswiederholung",
    where: "SDK · bevor eine 200 empfangen wird",
    trigger:
      "Keine Verbindung oder noch keine 200: Netzwerkfehler wie Verbindungsreset / Lese-Schreib-Timeout / DNS-Ausfall, plus HTTP 408, 429, 500, 502, 503, 504.",
    skips: "Alle anderen Statuscodes (400 / 401 / 403 / 404 / 413 / 422 usw.) sind deterministische Ablehnungen; erneutes Senden schlägt genauso fehl, daher werden sie sofort geworfen.",
    desc: "Sendet dieselbe Anfrage unverändert erneut. Sobald der Stream startet (eine 200 wurde empfangen), wird ein Stream-Abbruch nicht mehr von dieser Ebene behandelt.",
    attemptsLabel: "Wiederholungsversuche",
    defAttempts: 3,
    defInterval: "0.5s→1s→2s exponentiell (begrenzt auf 8s)",
    offHint: "-1 = nie wiederholen; bei Fehler sofort werfen",
  },
  empty: {
    title: "Leerantwort-Wiederholung",
    where: "SDK · nur OpenAI-Format",
    trigger:
      "HTTP 200 mit normalem Stopp-Grund, aber die gesamte Antwort enthält keinen Inhaltsblock — leere Gateway-Frames, verworfene Denkfelder und Sampling-Aussetzer sehen alle so aus.",
    skips: "Leerantworten durch max_tokens-Begrenzung zählen nicht (beheben durch Erhöhen des Ausgabelimits; erneutes Senden stößt wieder an dieselbe Grenze).",
    desc: "Sendet den gesamten Prompt erneut, was bei langen Kontexten relativ teuer ist; Anzahl gering halten.",
    attemptsLabel: "Wiederholungsversuche",
    defAttempts: 2,
    defInterval: "0.5s→1s→2s exponentiell (begrenzt auf 8s)",
    offHint: "-1 = leere Antwort unverändert zurückgeben",
  },
  stream: {
    title: "Same-Provider-Safe-Window-Wiederholung",
    where: "Dieses Projekt · bevor Ausgabe geliefert wird",
    trigger:
      "Probleme nach Stream-Aufbau (200 empfangen): Stream-Abbruch, Provider überlastet, 429/5xx-Fehler im Stream — und noch kein Token wurde an den Aufrufer geliefert.",
    skips:
      "Kontingent erschöpft (402 / insufficient_quota, wird durch Polling zum Profilwechsel behandelt), Kontext zu lang (413 / context length, wird durch Komprimierung behandelt), und deterministische 400/401/403/404/422-Ablehnungen werden nicht wiederholt.",
    desc: "Spielt dieselbe Anfrage auf demselben Profil erneut ab. Da noch keine Ausgabe geliefert wurde, dupliziert die Wiederholung weder Modellausgabe noch Tool-Ausführung.",
    attemptsLabel: "Wiederholungsversuche",
    defAttempts: 2,
    defInterval: "0.5s→1s exponentiell (begrenzt auf 4s)",
    offHint: "-1 = defekten Stream direkt an die äußere Intent-Wiederholung übergeben",
  },
  breaker: {
    title: "Polling-Breaker",
    where: "Dieses Projekt · prozessweit, eine globale Kopie",
    trigger:
      "Löst aus, wenn transient Fehler (429, 5xx, Netzwerkfehler) nacheinander den Schwellenwert erreichen; deterministische Fehler wie unzureichendes Guthaben (402), ungültiger Schlüssel (401/403) und fehlendes Modell (404) ignorieren den Schwellenwert und lösen sofort aus.",
    skips: "Ein Erfolg setzt den Zähler zurück, sodass ein gelegentlich hickendes Profil nicht langsam in einen Auslöser läuft.",
    desc: "Nach Auslösung geht das Profil in Abkühlphase, und Polling überspringt es für diese Zeit. Zustand wird persistiert und überlebt Neustarts.",
    attemptsLabel: "Aufeinanderfolgende Fehler vor Auslösung",
    defAttempts: 3,
    defInterval: "1min→5min→30min Gradient",
    offHint: "-1 = transient Fehler lösen nie aus (deterministische Fehler lösen trotzdem aus)",
  },
  intent: {
    title: "Intent-Wiederholung",
    where: "Dieses Projekt · prozessweit, eine globale Kopie",
    trigger:
      "Keine der früheren Ebenen hat es abgefangen: Der Worker endet mit model_error — entweder sind alle inneren Wiederholungen erschöpft, oder der Stream brach ab, nachdem Ausgabe bereits geliefert wurde (Wiederholung ist dann unsicher, also muss alles neu starten).",
    skips: "Kontingenterschöpfung wird bereits durch Polling zum Profilwechsel behandelt und hier nicht wiederholt; wenn eine Aufgabe pausiert / beendet / in den Abschluss geht, gibt sie sofort auf, ohne Backoff-Zeit zu verbrauchen.",
    desc: "Führt den gesamten Intent von Grund auf neu aus. Es ist die äußerste Ebene, sodass eine Wiederholung die Versuche der inneren Ebenen erneut multipliziert.",
    attemptsLabel: "Neuausführungsversuche",
    defAttempts: 2,
    defInterval: "Feste 3s",
    offHint: "-1 = nicht wiederholen; Intent direkt als blockiert markieren",
  },
} satisfies Record<string, LayerMeta>;

type LayerKey = keyof typeof RETRY_LAYERS;

/** Human-readable milliseconds, used only for echo beside the input so nobody has to count zeros. */
function humanMs(ms: number) {
  if (!Number.isFinite(ms) || ms <= 0) return "";
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${Number((ms / 1000).toFixed(2))}s`;
  return `${Number((ms / 60_000).toFixed(2))}min`;
}

/** Controlled numeric input: empty string ↔ 0; intermediate states ("-", "1e") stay local without bothering the parent. */
function NumField({
  id,
  value,
  onChange,
  placeholder,
  min,
}: {
  id: string;
  value: number;
  onChange: (n: number) => void;
  placeholder: string;
  min: number;
}) {
  const [text, setText] = React.useState(value === 0 ? "" : String(value));
  // Keep up when the parent swaps in a whole new set of values (policy loaded, profile switched); typing locally does not reach here,
  // because at that point value already equals the parsed local text.
  React.useEffect(() => {
    const incoming = value === 0 ? "" : String(value);
    setText((cur) => (Number(cur || 0) === value ? cur : incoming));
  }, [value]);
  return (
    <Input
      id={id}
      type="number"
      min={min}
      className="w-28 shrink-0"
      value={text}
      placeholder={placeholder}
      onChange={(e) => {
        setText(e.target.value);
        const n = Number(e.target.value);
        onChange(e.target.value.trim() === "" || !Number.isFinite(n) ? 0 : Math.trunc(n));
      }}
    />
  );
}

/** The two knobs of one retry layer. idPrefix keeps the label's htmlFor unique when rendered multiple times on one page. */
export function RetryRuleFields({
  layer,
  idPrefix,
  value,
  onChange,
  compact,
}: {
  layer: LayerKey;
  idPrefix: string;
  value: LLMRetryRule;
  onChange: (r: LLMRetryRule) => void;
  /** true = compact version in the profile drawer: omit the expanded explanation and keep only the "what errors reach this layer" line */
  compact?: boolean;
}) {
  const meta = RETRY_LAYERS[layer];
  const human = humanMs(value.interval_ms);
  return (
    <div className={compact ? "grid gap-2" : "grid gap-3 rounded-lg border p-3"}>
      <div className="grid gap-0.5">
        <div className="flex flex-wrap items-baseline gap-2">
          <Label className="text-sm">{meta.title}</Label>
          <span className="text-muted-foreground text-xs">{meta.where}</span>
        </div>
        {/* Which errors reach this layer, down to status codes — if a knob seems to have no effect, the error probably does not land here at all. */}
        <p className="text-muted-foreground text-xs">
          <span className="font-medium text-foreground">Auslöser</span>: {meta.trigger}
        </p>
        {!compact && meta.skips && (
          <p className="text-muted-foreground text-xs">
            <span className="font-medium text-foreground">Nutzt diese Ebene nicht</span>: {meta.skips}
          </p>
        )}
        {!compact && <p className="text-muted-foreground text-xs">{meta.desc}</p>}
      </div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="flex items-center gap-2">
          <Label htmlFor={`${idPrefix}-${layer}-n`} className="text-muted-foreground text-xs">
            {meta.attemptsLabel}
          </Label>
          <NumField
            id={`${idPrefix}-${layer}-n`}
            min={-1}
            value={value.attempts}
            placeholder={`Standard ${meta.defAttempts}`}
            onChange={(n) => onChange({ ...value, attempts: n })}
          />
        </div>
        <div className="flex items-center gap-2">
          <Label htmlFor={`${idPrefix}-${layer}-ms`} className="text-muted-foreground text-xs">
            Intervall ms
          </Label>
          <NumField
            id={`${idPrefix}-${layer}-ms`}
            min={0}
            value={value.interval_ms}
            placeholder="Standard-Backoff"
            onChange={(n) => onChange({ ...value, interval_ms: n })}
          />
          <span className="text-muted-foreground text-xs">{human ? `Fest ${human}` : meta.defInterval}</span>
        </div>
      </div>
      {!compact && <p className="text-muted-foreground text-xs">Leer = Standard verwenden; {meta.offHint}.</p>}
    </div>
  );
}

/** The three-layer override in the model profile drawer (the three layers tied to the endpoint). */
export function ProfileRetryFields({
  value,
  onChange,
}: {
  value: LLMRetryOverride;
  onChange: (o: LLMRetryOverride) => void;
}) {
  return (
    <div className="grid gap-3 rounded-lg border p-3">
      <div className="grid gap-0.5">
        <Label className="text-sm">Wiederholungen für dieses Profil</Label>
        <p className="text-muted-foreground text-xs">
          Diese Einstellungen gelten nur für dieses Profil. Leere Felder übernehmen die globalen Werte unter „Wiederholungen und Wartezeiten“. Mit −1 Versuchen wird die jeweilige Wiederholungsstufe deaktiviert. Ein festes Intervall ersetzt die exponentielle Wartezeit. Die Unterbrechung bei Fehlern und das Wiederholen von Absichten lassen sich nur global einstellen.
        </p>
      </div>
      {(["connect", "empty", "stream"] as const).map((k) => (
        <div key={k} className="border-t pt-3 first:border-t-0 first:pt-0">
          <RetryRuleFields
            compact
            layer={k}
            idPrefix="pf"
            value={value[k]}
            onChange={(r) => onChange({ ...value, [k]: r })}
          />
        </div>
      ))}
    </div>
  );
}

/** "Retry and backoff" tab: global defaults for the five layers. */
export function RetryPolicyPanel() {
  const [policy, setPolicy] = React.useState<LLMRetryPolicy>(ZERO_POLICY);
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const p = await api.llmRetryPolicy();
      setPolicy({ ...ZERO_POLICY, ...p });
    } catch (e) {
      toast.error(`Laden der Wiederholungsrichtlinie fehlgeschlagen: ${(e as Error).message}`);
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => {
    void load();
  }, [load]);

  async function save() {
    if (saving) return;
    setSaving(true);
    try {
      // The backend clamps out-of-range values and returns them; refresh directly with the returned values, so what you see is what is stored.
      const saved = await api.saveLLMRetryPolicy(policy);
      setPolicy({ ...ZERO_POLICY, ...saved });
      toast.success("Gespeichert und sofort wirksam (laufende Aufrufe nutzen noch die alten Parameter)");
    } catch (e) {
      toast.error(`Speichern fehlgeschlagen: ${(e as Error).message}`);
    } finally {
      setSaving(false);
    }
  }

  const set = (k: LayerKey) => (r: LLMRetryRule) => setPolicy((p) => ({ ...p, [k]: r }));

  if (loading) {
    return (
      <div className="flex items-center gap-2 rounded-lg border border-dashed p-10 text-muted-foreground text-sm">
        <Loader2Icon className="size-4 animate-spin" /> Lade Wiederholungsrichtlinie…
      </div>
    );
  }

  return (
    <div className="grid gap-4">
      <div className="rounded-lg border bg-muted/30 p-3 text-muted-foreground text-xs leading-relaxed">
        Ein fehlgeschlagener Modellaufruf durchläuft fünf Wiederholungsebenen der Reihe nach, von der innersten zur äußersten:
        <span className="text-foreground"> Verbindung → Leerantwort → Same-Provider-Safe-Window → Polling-Breaker → Intent-Wiederholung</span>
        . Eine äußere Ebene läuft erst, nachdem die innere erschöpft ist, sodass sich die Anzahlen
        <span className="text-foreground">multiplizieren</span>
        — werden alle Ebenen ausgeschöpft, kann ein Haken Dutzende Anfragen verbrennen.
        Wenn alles leer bleibt, gelten die aktuellen Standardwerte, identisch zum Verhalten ohne diese Seite. Die ersten drei Ebenen können pro Modellprofil überschrieben werden.
      </div>

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {(Object.keys(RETRY_LAYERS) as LayerKey[]).map((k) => (
          <RetryRuleFields key={k} layer={k} idPrefix="gl" value={policy[k]} onChange={set(k)} />
        ))}
      </div>

      <div className="flex gap-2">
        <Button onClick={save} disabled={saving}>
          {saving ? <Loader2Icon className="animate-spin" /> : <SaveIcon />}
          Speichern
        </Button>
        <Button variant="outline" onClick={() => setPolicy(ZERO_POLICY)} disabled={saving}>
          Alle Standardwerte wiederherstellen
        </Button>
      </div>
    </div>
  );
}
