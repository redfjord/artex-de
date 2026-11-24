"use client";

import * as React from "react";

import { CpuIcon, FlaskConicalIcon, KeyboardIcon, RadioTowerIcon, SearchIcon, ShieldAlertIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { api } from "@/lib/api";
import { CHAT_SEND_MODE_OPTIONS, type ChatSendMode, setChatSendMode, useChatSendMode } from "@/lib/chat-send-mode";
import type { Settings } from "@/lib/types";

import { UpdateCard } from "./_components/update-card";

export default function SystemSettingsPage() {
  const [trafficCapture, setTrafficCapture] = React.useState(false);
  const [agentTrafficBinding, setAgentTrafficBinding] = React.useState(false);
  const [webSearch, setWebSearch] = React.useState(false);
  const [backend, setBackend] = React.useState("ddgs");
  const [braveKeySet, setBraveKeySet] = React.useState(false);
  const [braveKeyInput, setBraveKeyInput] = React.useState("");
  const [tavilyKeySet, setTavilyKeySet] = React.useState(false);
  const [tavilyKeyInput, setTavilyKeyInput] = React.useState("");
  const [savingTavilyKey, setSavingTavilyKey] = React.useState(false);
  const [proxyInput, setProxyInput] = React.useState("");
  const [savingProxy, setSavingProxy] = React.useState(false);
  const [globalProxyInput, setGlobalProxyInput] = React.useState("");
  const [savingGlobalProxy, setSavingGlobalProxy] = React.useState(false);
  const [testing, setTesting] = React.useState(false);
  const [loaded, setLoaded] = React.useState(false);
  const [saving, setSaving] = React.useState(false);
  const [savingKey, setSavingKey] = React.useState(false);
  const [pyInterp, setPyInterp] = React.useState("");
  const [workers, setWorkers] = React.useState("3");
  const [savingWorkers, setSavingWorkers] = React.useState(false);
  // Scope of operation-constraint injection (both on by default).
  const [injectPlanner, setInjectPlanner] = React.useState(true);
  const [injectWorker, setInjectWorker] = React.useState(true);
  // Experimental feature: noa context compaction (off by default).
  const [noaCompaction, setNoaCompaction] = React.useState(false);
  // Frontend-only preference: bypasses /api/settings and reads/writes localStorage directly.
  const sendMode = useChatSendMode();

  const apply = React.useCallback((s: Settings) => {
    setTrafficCapture(!!s.traffic_capture);
    setAgentTrafficBinding(!!s.agent_traffic_binding);
    setWebSearch(!!s.web_search_enabled);
    setBackend(s.web_search_backend || "ddgs");
    setBraveKeySet(!!s.brave_key_set);
    setTavilyKeySet(!!s.tavily_key_set);
    setProxyInput(s.web_search_proxy ?? "");
    setGlobalProxyInput(s.global_proxy ?? "");
    setPyInterp(s.python_interpreter ?? "");
    setWorkers(String(s.workers ?? 3));
    setInjectPlanner(s.constraints_inject_planner !== false);
    setInjectWorker(s.constraints_inject_worker !== false);
    setNoaCompaction(!!s.noa_compaction);
  }, []);

  const saveWorkers = () => {
    const n = Number(workers);
    if (!Number.isInteger(n) || n <= 0) {
      toast.error("Parallelität muss eine ganze Zahl größer als 0 sein");
      return;
    }
    setSavingWorkers(true);
    api
      .setSettings({ workers: n })
      .then((s) => {
        apply(s);
        toast.success("Worker-Agent-Parallelität gespeichert (gilt für später gestartete Aufgaben)");
      })
      .catch((e) => toast.error("Speichern fehlgeschlagen: " + (e as Error).message))
      .finally(() => setSavingWorkers(false));
  };

  const savePython = () => {
    setSaving(true);
    api
      .setSettings({ python_interpreter: pyInterp.trim() })
      .then((s) => {
        apply(s);
        toast.success("Python-Interpreter-Konfiguration gespeichert");
      })
      .catch((e) => toast.error("Speichern fehlgeschlagen: " + (e as Error).message))
      .finally(() => setSaving(false));
  };
  const detectPython = () => {
    setSaving(true);
    api
      .detectPython()
      .then((r) => setPyInterp(r.python_interpreter))
      .catch(() => undefined)
      .finally(() => setSaving(false));
  };

  React.useEffect(() => {
    api
      .settings()
      .then(apply)
      .catch(() => undefined)
      .finally(() => setLoaded(true));
  }, [apply]);

  const toggleTraffic = (v: boolean) => {
    setTrafficCapture(v); // optimistic
    setSaving(true);
    api
      .setSettings({ traffic_capture: v })
      .then(apply)
      .catch(() => setTrafficCapture(!v)) // revert on failure
      .finally(() => setSaving(false));
  };

  const toggleInjectPlanner = (v: boolean) => {
    setInjectPlanner(v); // optimistic
    api
      .setSettings({ constraints_inject_planner: v })
      .then(apply)
      .catch(() => setInjectPlanner(!v)); // revert on failure
  };

  const toggleAgentTrafficBinding = (v: boolean) => {
    setAgentTrafficBinding(v);
    setSaving(true);
    api
      .setSettings({ agent_traffic_binding: v })
      .then((s) => {
        apply(s);
        toast.success(v ? "Agenten-Datenverkehr-Autobindung aktiviert" : "Agenten-Datenverkehr-Autobindung deaktiviert");
      })
      .catch((e) => {
        setAgentTrafficBinding(!v);
        toast.error(`Speichern fehlgeschlagen: ${(e as Error).message}`);
      })
      .finally(() => setSaving(false));
  };

  const toggleInjectWorker = (v: boolean) => {
    setInjectWorker(v); // optimistic
    api
      .setSettings({ constraints_inject_worker: v })
      .then(apply)
      .catch(() => setInjectWorker(!v)); // revert on failure
  };

  const toggleNoaCompaction = (v: boolean) => {
    setNoaCompaction(v); // optimistic
    api
      .setSettings({ noa_compaction: v })
      .then((s) => {
        apply(s);
        toast.success(v ? "noa-Kontextkomprimierung aktiviert (gilt für später gestartete Läufe)" : "noa-Kontextkomprimierung deaktiviert (eingebaute Komprimierung wiederhergestellt)");
      })
      .catch((e) => {
        setNoaCompaction(!v); // revert on failure
        toast.error(`Speichern fehlgeschlagen: ${(e as Error).message}`);
      });
  };

  // Persist a web-search patch (enable and/or backend). Optimistic with refetch.
  const saveWebSearch = (patch: Partial<Settings>) => {
    setSaving(true);
    api
      .setSettings(patch)
      .then((s) => {
        apply(s);
        toast.success("Websuche-Konfiguration gespeichert");
      })
      .catch((e) => {
        toast.error("Speichern fehlgeschlagen: " + (e as Error).message);
        api
          .settings()
          .then(apply)
          .catch(() => undefined);
      })
      .finally(() => setSaving(false));
  };

  const saveBraveKey = () => {
    setSavingKey(true);
    api
      .setSettings({ brave_search_api_key: braveKeyInput })
      .then((s) => {
        apply(s);
        setBraveKeyInput("");
        toast.success("Brave API-Schlüssel gespeichert");
      })
      .catch((e) => toast.error("Speichern fehlgeschlagen: " + (e as Error).message))
      .finally(() => setSavingKey(false));
  };

  const saveTavilyKey = () => {
    setSavingTavilyKey(true);
    api
      .setSettings({ tavily_search_api_key: tavilyKeyInput })
      .then((s) => {
        apply(s);
        setTavilyKeyInput("");
        toast.success("Tavily API-Schlüssel gespeichert");
      })
      .catch((e) => toast.error("Speichern fehlgeschlagen: " + (e as Error).message))
      .finally(() => setSavingTavilyKey(false));
  };

  const saveProxy = () => {
    setSavingProxy(true);
    api
      .setSettings({ web_search_proxy: proxyInput.trim() })
      .then((s) => {
        apply(s);
        toast.success(proxyInput.trim() ? "Egress-Proxy gespeichert" : "Egress-Proxy gelöscht (direkte Verbindung wird verwendet)");
      })
      .catch((e) => toast.error("Speichern fehlgeschlagen: " + (e as Error).message))
      .finally(() => setSavingProxy(false));
  };

  const saveGlobalProxy = () => {
    setSavingGlobalProxy(true);
    api
      .setSettings({ global_proxy: globalProxyInput.trim() })
      .then((s) => {
        apply(s);
        toast.success(globalProxyInput.trim() ? "Globaler Proxy gespeichert" : "Globaler Proxy gelöscht (direkte Verbindung wird verwendet)");
      })
      .catch((e) => toast.error("Speichern fehlgeschlagen: " + (e as Error).message))
      .finally(() => setSavingGlobalProxy(false));
  };

  // Run a real "test" search ("test") against the CURRENT form values (backend +
  // proxy + entered key), falling back to saved values server-side. Toasts result.
  const runTest = () => {
    setTesting(true);
    api
      .testWebSearch({
        web_search_backend: backend,
        web_search_proxy: proxyInput.trim(),
        brave_search_api_key: braveKeyInput,
        tavily_search_api_key: tavilyKeyInput,
      })
      .then((r) => {
        if (r.ok) toast.success(`Suchtest erfolgreich · ${r.backend} lieferte ${r.count} Ergebnisse`);
        else toast.error("Suchtest fehlgeschlagen: " + (r.error || "Unknown error"));
      })
      .catch((e) => toast.error("Suchtest fehlgeschlagen: " + (e as Error).message))
      .finally(() => setTesting(false));
  };

  // brave-free selected but no key stored and none being entered → tool stays off.
  const braveNeedsKey = webSearch && backend === "brave-free" && !braveKeySet;

  return (
    <div className="flex flex-1 flex-col gap-4 md:gap-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">Systemeinstellungen</h1>
        <p className="text-muted-foreground text-sm">Globale Laufzeitschalter</p>
      </div>

      {/* Multi-column rather than grid: the web search card is several times taller than the
          others and its height varies with the selected backend (brave/tavily key inputs are
          conditionally rendered). Grid would stretch the whole row to the tallest card and leave
          large gaps beside it, while multi-column fills by content height automatically. Card
          spacing uses mb rather than gap — in a multi-column layout column-gap only handles
          column spacing; row spacing must come from the children. */}
      <div className="columns-1 gap-4 md:gap-6 lg:columns-2">
        <UpdateCard />

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <RadioTowerIcon className="size-4" />
              Datenverkehr-Aufzeichnung
            </CardTitle>
            <CardDescription>
              When on, all agent HTTP traffic is fully stored via the recording proxy, and the
              traffic_search / traffic_get tools plus proxy configuration are injected into agents
              (the prompt includes proxy instructions).
              <br />
              When off (default), no traffic is recorded: agents
              <b> do not</b> receive the proxy configuration or traffic tools, and the prompt <b>does not</b> mention the proxy. Toggling rebuilds agents immediately.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex items-center justify-between gap-4">
            <Label htmlFor="traffic-capture" className="text-sm font-normal text-muted-foreground">
              {trafficCapture ? "Ein · zeichnet Datenverkehr auf und injiziert Proxy" : "Aus · keine Aufzeichnung, keine Proxy-Injektion"}
            </Label>
            <Switch
              id="traffic-capture"
              checked={trafficCapture}
              disabled={!loaded || saving}
              onCheckedChange={toggleTraffic}
            />
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <RadioTowerIcon className="size-4" />
              Agenten-Datenverkehr-Autobindung
            </CardTitle>
            <CardDescription id="agent-traffic-binding-description">
              Off by default. When on, the report agent triggered on finding ingestion checks existing HTTP
              requests/responses and links the matching traffic before writing the report.
              <b>Inspecting packets and making extra tool calls increases token usage.</b>
              <br />
              TCP, uncaptured traffic, or no matching traffic can still be reported normally. This switch does
              not affect traffic capture, manual binding, or viewing saved evidence. It applies to the next agent
              round; once disabled, new automatic bindings are rejected immediately.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex items-center justify-between gap-4">
            <Label htmlFor="agent-traffic-binding" className="text-sm font-normal text-muted-foreground">
              {agentTrafficBinding ? "Ein · erhöht Token-Verbrauch" : "Aus · manuelle Bindung weiterhin verfügbar"}
            </Label>
            <Switch
              id="agent-traffic-binding"
              aria-describedby="agent-traffic-binding-description"
              checked={agentTrafficBinding}
              disabled={!loaded || saving}
              onCheckedChange={toggleAgentTrafficBinding}
            />
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <RadioTowerIcon className="size-4" />
              Globaler Proxy
            </CardTitle>
            <CardDescription>
              All agents' <b>target traffic</b> egresses through this proxy (hides the source IP / uses a jump host). Supports <b>http / https / socks5</b>, with optional{" "}
              <code>user:pass</code> auth. Leave blank for a direct connection.
              <br />
              When <b>traffic capture</b> is on, it acts as the recording proxy's <b>upstream</b> (traffic is still fully stored, then egresses through this proxy); when capture is off, it is injected directly into agents' bash / WebFetch egress. Independent of the web search proxy and LLM proxy.
              <br />
              <b>Tip</b>: with capture <b>off</b>, socks5 depends on each command-line tool supporting <code>ALL_PROXY</code> (curl
              works; some tools may ignore it). If you mainly use socks5, enabling traffic capture is recommended — that path is dialed directly by the MITM, invisible to tools and reliably effective.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            <Label htmlFor="global-proxy" className="text-sm font-normal text-muted-foreground">
              Proxy-Adresse
            </Label>
            <div className="flex items-center gap-2">
              <Input
                id="global-proxy"
                autoComplete="off"
                placeholder="socks5://user:pass@host:1080 oder http://host:port (leer für direkte Verbindung)"
                value={globalProxyInput}
                disabled={!loaded || savingGlobalProxy}
                onChange={(e) => setGlobalProxyInput(e.target.value)}
              />
              <Button type="button" onClick={saveGlobalProxy} disabled={!loaded || savingGlobalProxy}>
                Speichern
              </Button>
            </div>
            <p className="text-muted-foreground text-xs">
              {globalProxyInput.trim() ? "Konfiguriert · aller Zieldatenverkehr läuft über diesen Proxy" : "Nicht konfiguriert · Zieldatenverkehr verbindet direkt"}
            </p>
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <ShieldAlertIcon className="size-4" />
              Injektion von Operationsbeschränkungen
            </CardTitle>
            <CardDescription>
              When on, each task's <b>operation constraints</b> (allow/deny entries maintained under "Operation
              constraints" in the task overview) are appended to the corresponding agent's system prompt to frame
              exploration boundaries (e.g. "only test the current port", "no brute forcing").
              <br />
              Injection into the <b>planner</b> and <b>worker</b> can be controlled separately; both are on by
              default. Changes take effect immediately (read on the next round) without rebuilding agents. When off, that agent no longer sees the constraints.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex items-center justify-between gap-4">
              <Label htmlFor="inject-planner" className="text-sm font-normal text-muted-foreground">
                In Planer injizieren{injectPlanner ? " · on" : " · off"}
              </Label>
              <Switch
                id="inject-planner"
                checked={injectPlanner}
                disabled={!loaded}
                onCheckedChange={toggleInjectPlanner}
              />
            </div>
            <div className="flex items-center justify-between gap-4">
              <Label htmlFor="inject-worker" className="text-sm font-normal text-muted-foreground">
                In Worker injizieren{injectWorker ? " · on" : " · off"}
              </Label>
              <Switch
                id="inject-worker"
                checked={injectWorker}
                disabled={!loaded}
                onCheckedChange={toggleInjectWorker}
              />
            </div>
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <FlaskConicalIcon className="size-4" />
              Experimentelle Funktionen
            </CardTitle>
            <CardDescription>
              Mechanisms still under validation, off by default. They may change agent behavior or affect
              stability; enable them only after understanding the impact.
              <br />
              <b>noa-Kontextkomprimierung</b>: the model actively compacts long conversation history (norma v0.4.0). When on, the four agent types on the platform (
              <b>planner / worker / main agent / chat</b>) hand context over to noa instead of built-in compaction,
              and the compacted originals are archived in the task working directory for review. Changes take effect immediately (for runs started later) without rebuilding agents;
              turning it off restores built-in compaction.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex items-center justify-between gap-4">
            <Label htmlFor="noa-compaction" className="text-sm font-normal text-muted-foreground">
              noa context compaction{noaCompaction ? " · on" : " · off"}
            </Label>
            <Switch
              id="noa-compaction"
              checked={noaCompaction}
              disabled={!loaded}
              onCheckedChange={toggleNoaCompaction}
            />
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <SearchIcon className="size-4" />
              Websuche
            </CardTitle>
            <CardDescription>
              This is the web search <b>master switch and source configuration</b>. Only when it is on can each
              <b> agent's configuration</b> individually enable <b>web_search</b> (returns titles/links/snippets
              only, not page bodies; fetching is handled by WebFetch). Web search <b>does not</b> go through the
              recording proxy and is independent of traffic capture.
              <br />
              Sources: <b>DuckDuckGo (ddgs)</b> (no key needed), <b>Brave (free tier)</b> (requires a Brave API key),{" "}
              <b>Tavily</b> (requires a Tavily API key), or <b>DeepSeek</b> (reuses the current LLM configuration). When the master switch is off, each
              agent's web search toggle is unavailable.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex items-center justify-between gap-4">
              <Label htmlFor="web-search" className="text-sm font-normal text-muted-foreground">
                {webSearch ? "Hauptschalter ein · pro Agent in jeweiliger Konfiguration aktivierbar" : "Aus · Agenten können Websuche nicht aktivieren"}
              </Label>
              <Switch
                id="web-search"
                checked={webSearch}
                disabled={!loaded || saving}
                onCheckedChange={(v) => {
                  setWebSearch(v); // optimistic
                  saveWebSearch({ web_search_enabled: v });
                }}
              />
            </div>

            {webSearch && (
              <div className="flex items-center justify-between gap-4">
                <Label className="text-sm font-normal text-muted-foreground">Suchquelle</Label>
                <Select
                  value={backend}
                  disabled={!loaded || saving}
                  onValueChange={(v) => {
                    setBackend(v); // optimistic
                    saveWebSearch({ web_search_backend: v });
                  }}
                >
                  <SelectTrigger className="w-48 shrink-0">
                    <SelectValue placeholder="Quelle auswählen" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ddgs">DuckDuckGo (ddgs · kostenlos, kein Schlüssel)</SelectItem>
                    <SelectItem value="brave-free">Brave (kostenloser Tarif · Schlüssel erforderlich)</SelectItem>
                    <SelectItem value="tavily">Tavily (Schlüssel erforderlich)</SelectItem>
                    <SelectItem value="deepseek">DeepSeek (offiziell)</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            )}

            {webSearch && backend === "deepseek" && (
              <div className="border-border/60 bg-muted/30 flex flex-col gap-2 rounded-md border p-3">
                <p className="text-sm font-medium">Offizielle DeepSeek-Websuche</p>
                <p className="text-muted-foreground text-xs leading-relaxed">
                  This source reuses the <b>currently active LLM configuration</b> directly. It therefore
                  <b> supports only official DeepSeek models</b>, and that configuration <b>must use the anthropic protocol</b>
                  — DeepSeek's OpenAI-protocol endpoint does not support server-side search. Switching LLM configurations may break this source.
                </p>
                <p className="text-muted-foreground text-xs leading-relaxed">
                  Unlike other sources, search is <b>executed by DeepSeek's servers</b>: each search consumes an extra model call
                  (incurring token costs), search requests <b>do not go through the egress proxy above</b>, and are <b>not recorded as traffic</b>; results contain <b>only titles and links</b>
                  (no snippets), so use WebFetch to fetch page bodies.
                </p>
                <p className="text-muted-foreground text-xs leading-relaxed">
                  Sie sind verantwortlich für die Bestätigung, dass diese Bedingungen erfüllt sind; das System erzwingt sie nicht. Nutzen Sie den Button „Testsuche“ unten, um eine echte Suche durchzuführen.
                </p>
              </div>
            )}

            {webSearch && backend === "brave-free" && (
              <div className="flex flex-col gap-2">
                <Label htmlFor="brave-key" className="text-sm font-normal text-muted-foreground">
                  Brave Search API-Schlüssel
                  {braveKeySet && <span className="ml-2 text-xs text-emerald-500">Konfiguriert</span>}
                </Label>
                <div className="flex items-center gap-2">
                  <Input
                    id="brave-key"
                    type="password"
                    autoComplete="off"
                    placeholder={braveKeySet ? "Konfiguriert (leer lassen zum Beibehalten)" : "Brave API-Schlüssel eingeben"}
                    value={braveKeyInput}
                    disabled={!loaded || savingKey}
                    onChange={(e) => setBraveKeyInput(e.target.value)}
                  />
                  <Button
                    type="button"
                    onClick={saveBraveKey}
                    disabled={!loaded || savingKey || braveKeyInput.trim() === ""}
                  >
                    Speichern
                  </Button>
                </div>
                {braveNeedsKey && (
                  <p className="text-xs text-amber-500">
                    Brave ist ausgewählt, aber noch kein Schlüssel konfiguriert — das Suchwerkzeug bleibt deaktiviert, bis Sie einen Schlüssel speichern.
                  </p>
                )}
                <p className="text-muted-foreground text-xs">
                  Der kostenlose Tarif erlaubt ca. 2.000 Abfragen/Monat. Schlüssel unter https://brave.com/search/api/ erhalten.
                </p>
              </div>
            )}

            {webSearch && backend === "tavily" && (
              <div className="flex flex-col gap-2">
                <Label htmlFor="tavily-key" className="text-sm font-normal text-muted-foreground">
                  Tavily Search API-Schlüssel
                  {tavilyKeySet && <span className="ml-2 text-xs text-emerald-500">Konfiguriert</span>}
                </Label>
                <div className="flex items-center gap-2">
                  <Input
                    id="tavily-key"
                    type="password"
                    autoComplete="off"
                    placeholder={tavilyKeySet ? "Konfiguriert (leer lassen zum Beibehalten)" : "Tavily API-Schlüssel eingeben (tvly-…)"}
                    value={tavilyKeyInput}
                    disabled={!loaded || savingTavilyKey}
                    onChange={(e) => setTavilyKeyInput(e.target.value)}
                  />
                  <Button
                    type="button"
                    onClick={saveTavilyKey}
                    disabled={!loaded || savingTavilyKey || tavilyKeyInput.trim() === ""}
                  >
                    Speichern
                  </Button>
                </div>
                {webSearch && backend === "tavily" && !tavilyKeySet && (
                  <p className="text-xs text-amber-500">
                    Tavily ist ausgewählt, aber noch kein Schlüssel konfiguriert — das Suchwerkzeug bleibt deaktiviert, bis Sie einen Schlüssel speichern.
                  </p>
                )}
                <p className="text-muted-foreground text-xs">Registrieren Sie sich unter https://tavily.com für einen API-Schlüssel.</p>
              </div>
            )}

            {webSearch && (
              <div className="flex flex-col gap-2">
                <Label htmlFor="ws-proxy" className="text-sm font-normal text-muted-foreground">
                  Egress-Proxy (optional)
                </Label>
                <div className="flex items-center gap-2">
                  <Input
                    id="ws-proxy"
                    autoComplete="off"
                    placeholder="http://host:port oder socks5://host:port (leer für direkte Verbindung)"
                    value={proxyInput}
                    disabled={!loaded || savingProxy}
                    onChange={(e) => setProxyInput(e.target.value)}
                  />
                  <Button type="button" onClick={saveProxy} disabled={!loaded || savingProxy}>
                    Speichern
                  </Button>
                </div>
                <p className="text-muted-foreground text-xs">
                  Dedizierter Egress-Proxy nur für Suchendpunkte (VPN/SOCKS usw.). Unabhängig vom Traffic-Aufzeichnungs-MITM-Proxy; verwenden, wenn direkter Zugriff fehlschlägt.
                </p>
              </div>
            )}

            {webSearch && (
              <div className="flex items-center justify-between gap-4 border-t pt-4">
                <p className="text-muted-foreground text-xs">
                  Führt eine echte Suche nach „test“ mit der aktuellen Konfiguration (Quelle + Proxy + Schlüssel) durch, um die Funktion zu prüfen.
                </p>
                <Button
                  type="button"
                  variant="outline"
                  onClick={runTest}
                  disabled={!loaded || testing}
                  className="shrink-0"
                >
                  {testing ? "Teste…" : "Testsuche"}
                </Button>
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <RadioTowerIcon className="size-4" />
              Benutzerdefinierte Skripte · Python-Interpreter
            </CardTitle>
            <CardDescription>
              Custom <b>script</b>-type tools use this to run Python. It is detected automatically at startup
              (python3 preferred); you can enter an absolute path to a venv or a specific version here, or leave it blank for runtime detection.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="flex items-center gap-2">
              <Input
                className="font-mono text-sm"
                placeholder="/usr/bin/python3 (leer für Auto-Erkennung)"
                value={pyInterp}
                disabled={!loaded || saving}
                onChange={(e) => setPyInterp(e.target.value)}
              />
              <Button variant="outline" onClick={detectPython} disabled={!loaded || saving}>
                Erneut erkennen
              </Button>
              <Button onClick={savePython} disabled={!loaded || saving}>
                Speichern
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <CpuIcon className="size-4" />
              Parallelität · Worker-Agenten
            </CardTitle>
            <CardDescription>
              Number of worker agents running concurrently per task (default 3). Higher values mean more concurrent probing and higher cost. Changes
              <b> apply to tasks started later</b>; running tasks are unaffected.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="flex items-center gap-2">
              <Input
                type="number"
                min={1}
                className="w-32 font-mono text-sm"
                placeholder="3"
                value={workers}
                disabled={!loaded || savingWorkers}
                onChange={(e) => setWorkers(e.target.value)}
              />
              <Button onClick={saveWorkers} disabled={!loaded || savingWorkers}>
                Speichern
              </Button>
            </div>
          </CardContent>
        </Card>

        <Card className="mb-4 break-inside-avoid md:mb-6">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <KeyboardIcon className="size-4" />
              Chat-Eingabe-Sendetaste
            </CardTitle>
            <CardDescription>
              Gilt für die Chatseite und das Eingabefeld der Hauptsitzung in den Aufgabendetails. Änderungen gelten sofort.
              <br />
              Diese Einstellung wird <b>nur in diesem Browser gespeichert</b>. Nach einem Browserwechsel oder dem Löschen der Websitedaten müssen Sie sie erneut festlegen.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex items-center justify-between gap-4">
            <Label htmlFor="chat-send-mode" className="text-sm font-normal text-muted-foreground">
              Sendeart
            </Label>
            <Select value={sendMode} onValueChange={(v) => setChatSendMode(v as ChatSendMode)}>
              <SelectTrigger id="chat-send-mode" className="w-72">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {CHAT_SEND_MODE_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
