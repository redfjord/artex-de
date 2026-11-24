"use client";

import * as React from "react";

import {
  CheckCircle2Icon,
  DownloadIcon,
  ExternalLinkIcon,
  RefreshCwIcon,
  RotateCcwIcon,
  TriangleAlertIcon,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { api, sseUrl } from "@/lib/api";
import type { UpdateCheck, UpdateProgress } from "@/lib/types";

/** Maximum time to wait for the new version to come up. An upgrade involves three
 *  process starts (stage -> swap -> new version), each taking seconds; three minutes
 *  covers slow disks and Docker container rebuilds. */
const RESTART_TIMEOUT_MS = 180_000;

function humanSize(n?: number): string {
  if (!n || n <= 0) return "";
  const units = ["B", "KB", "MB", "GB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

export function UpdateCard() {
  const [info, setInfo] = React.useState<UpdateCheck | null>(null);
  const [checking, setChecking] = React.useState(true);
  const [progress, setProgress] = React.useState<UpdateProgress | null>(null);
  // Separate from progress: once staging completes the process is gone and SSE drops, so we switch to polling /api/health.
  const [restarting, setRestarting] = React.useState(false);
  const [busy, setBusy] = React.useState(false);

  // quiet also decides whether to bypass the backend cache: the automatic check on page
  // load uses the cache (the top bar just checked), while a manual "Nach Updates suchen"
  // forces a fresh fetch — otherwise a just-released version would not show until the cache expires.
  const check = React.useCallback((quiet = false) => {
    setChecking(true);
    api
      .checkUpdate(!quiet)
      .then((r) => {
        setInfo(r);
        if (!quiet) {
          if (r.error) toast.error("Fehler beim Prüfen auf Updates: " + r.error);
          else if (r.has_update) toast.success(`Neue Version ${r.latest} verfügbar`);
          else if (r.comparable) toast.success("Sie verwenden die neueste Version");
        }
      })
      .catch((e) => {
        if (!quiet) toast.error("Fehler beim Prüfen auf Updates: " + (e as Error).message);
      })
      .finally(() => setChecking(false));
  }, []);

  React.useEffect(() => {
    check(true);
  }, [check]);

  // Poll /api/health until the version number changes.
  //
  // The criterion must be "the version changed", not "the service is reachable": during the
  // swap the old version briefly starts once more (that run only swaps in artex.new and exits
  // immediately), so checking connectivity alone would falsely report success.
  const waitForNewVersion = React.useCallback(async (fromVersion: string) => {
    setRestarting(true);
    const deadline = Date.now() + RESTART_TIMEOUT_MS;
    while (Date.now() < deadline) {
      await sleep(2000);
      try {
        const r = await fetch("/api/health", { cache: "no-store" });
        if (r.ok) {
          const j = (await r.json()) as { version?: string };
          if (j.version && j.version !== fromVersion) {
            toast.success(`Updated to ${j.version}, reloading the page`);
            await sleep(800);
            window.location.reload();
            return;
          }
        }
      } catch {
        // Being unreachable during the restart window is expected; keep polling.
      }
    }
    setRestarting(false);
    toast.error("Zeitüberschreitung beim Warten auf den Neustart des Dienstes. Prüfen Sie die Backend-Logs und stellen Sie sicher, dass artex über start.sh / start.bat gestartet wurde.");
  }, []);

  // Subscribe to update progress. SSE does not go through Next's /api rewrite (it buffers, so events never get through).
  const openStream = React.useCallback(
    (fromVersion: string) => {
      const es = new EventSource(sseUrl("/api/update/stream"));
      es.onmessage = (ev) => {
        let p: UpdateProgress;
        try {
          p = JSON.parse(ev.data) as UpdateProgress;
        } catch {
          return;
        }
        setProgress(p);
        if (p.phase === "failed") {
          es.close();
          setBusy(false);
          toast.error("Update fehlgeschlagen: " + (p.error || p.message));
          return;
        }
        if (p.phase === "staged") {
          es.close();
          void waitForNewVersion(fromVersion);
        }
      };
      es.onerror = () => {
        // SSE necessarily drops when the process exits. If we are already waiting for a restart
        // this is normal; let the /api/health polling decide.
        es.close();
      };
      return es;
    },
    [waitForNewVersion],
  );

  const doUpdate = () => {
    if (!info) return;
    const from = info.current;
    const ok = window.confirm(
      `Auf ${info.latest} aktualisieren?\n\n` +
        "Bei der Aktualisierung wird das Programm neu gestartet; laufende Aufgaben werden unterbrochen.\n" +
        (info.mode === "docker"
          ? "\nHinweis: In-Container-Aktualisierungen ersetzen nur das Programm selbst und aktualisieren keine Tools wie playwright / nmap im Image; " +
            "wenn die neue Version von neuen Tools abhängt, verwenden Sie stattdessen docker compose pull."
          : ""),
    );
    if (!ok) return;

    setBusy(true);
    setProgress({ phase: "downloading", percent: 0, message: "Wird vorbereitet…" });
    const es = openStream(from);
    api.applyUpdate().catch((e) => {
      es.close();
      setBusy(false);
      setProgress(null);
      toast.error("Start des Updates fehlgeschlagen: " + (e as Error).message);
    });
  };

  const doRollback = () => {
    if (!info) return;
    if (
      !window.confirm(
        "Zur vorherigen Version zurückkehren?\n\nDas Programm wird neu gestartet; laufende Aufgaben werden unterbrochen.\nHinweis: Das Datenbankschema wird nicht zurückgesetzt, daher kann die alte Version möglicherweise Daten, die von der neuen Version geschrieben wurden, nicht verstehen.",
      )
    )
      return;
    const from = info.current;
    setBusy(true);
    api
      .rollbackUpdate()
      .then(() => {
        toast.success("Zur vorherigen Version gewechselt, wird neu gestartet…");
        void waitForNewVersion(from);
      })
      .catch((e) => {
        setBusy(false);
        toast.error("Rollback fehlgeschlagen: " + (e as Error).message);
      });
  };

  const phase = progress?.phase;
  const showProgress = busy || restarting;
  // Only the download phase has a real percentage (computed from Content-Length). Verification,
// extraction, and waiting for a restart have unknown durations, so fill the bar and pulse it to
// indicate "busy, but no reliable estimate".
  const downloading = !restarting && phase === "downloading";
  const pct = downloading ? Math.max(progress?.percent ?? 0, 0) : 100;

  return (
    // The settings page is a multi-column masonry layout; each card handles its own row spacing
    // and must not break across columns (see the comment in page.tsx).
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <DownloadIcon className="size-4" />
          Version & Updates
        </CardTitle>
        <CardDescription>Nach neuen Versionen auf GitHub suchen und diese installieren. Bei der Aktualisierung wird das Programm neu gestartet; laufende Aufgaben werden unterbrochen.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <span className="text-muted-foreground">Aktuelle Version</span>
          <Badge variant="secondary" className="font-mono">
            {info?.current ?? "…"}
          </Badge>
          {info && (
            <>
              <Badge variant="outline" className="font-mono">
                {info.os}/{info.arch}
              </Badge>
              <Badge variant="outline">{info.mode === "docker" ? "Docker" : "Standalone"}</Badge>
            </>
          )}
          {info?.latest && (
            <>
              <span className="text-muted-foreground">Neueste Version</span>
              <Badge variant={info.has_update ? "default" : "secondary"} className="font-mono">
                {info.latest}
              </Badge>
            </>
          )}
          {info?.html_url && (
            <a
              href={info.html_url}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 text-xs text-muted-foreground underline-offset-4 hover:underline"
            >
              Versionshinweise <ExternalLinkIcon className="size-3" />
            </a>
          )}
        </div>

        {info?.boot_notice && (
          <p className="flex items-start gap-2 rounded-md border border-amber-500/40 bg-amber-500/10 p-2 text-xs text-amber-700 dark:text-amber-400">
            <TriangleAlertIcon className="mt-0.5 size-3.5 shrink-0" />
            {info.boot_notice}
          </p>
        )}

        {info?.error && (
          <p className="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 p-2 text-xs text-destructive">
            <TriangleAlertIcon className="mt-0.5 size-3.5 shrink-0" />
            GitHub nicht erreichbar: {info.error}
            {" "}Einen globalen Proxy oben konfigurieren und erneut versuchen.
          </p>
        )}

        {info && !info.comparable && info.reason && <p className="text-xs text-muted-foreground">{info.reason}</p>}

        {info?.has_update && info.asset_available === false && (
          <p className="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 p-2 text-xs text-destructive">
            <TriangleAlertIcon className="mt-0.5 size-3.5 shrink-0" />
            {info.latest} hat kein Release-Paket für {info.os}/{info.arch} (fehlt {info.asset}); automatische Aktualisierung nicht verfügbar.
          </p>
        )}

        {info?.has_update && info.asset_available !== false && (
          <p className="text-xs text-muted-foreground">
            Wird heruntergeladen <span className="font-mono">{info.asset}</span>
            {info.size ? ` (${humanSize(info.size)})` : ""}; es wird erst nach SHA256-Verifizierung und einem Smoketest ersetzt, und die aktuelle Version wird bei Fehlschlag beibehalten.
          </p>
        )}

        {info && !info.has_update && info.comparable && !info.error && (
          <p className="flex items-center gap-2 text-xs text-muted-foreground">
            <CheckCircle2Icon className="size-3.5 text-emerald-600" />
            Sie verwenden die neueste Version.
          </p>
        )}

        {info?.mode === "docker" && info.has_update && (
          <p className="text-xs text-muted-foreground">
            Unter Docker ersetzt eine Aktualisierung nur das Programm selbst, nicht die Tools wie playwright / nmap im Image, und
            <span className="font-mono"> docker compose up -d </span>
            setzt nach dem Neuerstellen des Containers auf die im Image enthaltene Version zurück. Um auch das Image zu aktualisieren, führen Sie aus
            <span className="font-mono"> docker compose pull artex &amp;&amp; docker compose up -d artex</span>.
          </p>
        )}

        {showProgress && (
          <div className="space-y-1.5">
            <Progress value={pct} className={downloading ? undefined : "animate-pulse"} />
            <p className="text-xs text-muted-foreground">
              {restarting ? "Neustart und Anwendung der neuen Version, bitte warten (die Seite wird automatisch neu geladen)…" : progress?.message}
            </p>
          </div>
        )}

        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" onClick={() => check(false)} disabled={checking || busy || restarting}>
            <RefreshCwIcon className={checking ? "size-4 animate-spin" : "size-4"} />
            Nach Updates suchen
          </Button>
          <Button
            size="sm"
            onClick={doUpdate}
            disabled={busy || restarting || !info?.has_update || info?.asset_available === false}
          >
            <DownloadIcon className="size-4" />
            {info?.has_update ? `Auf ${info.latest} aktualisieren` : "Jetzt aktualisieren"}
          </Button>
          {info?.has_backup && (
            <Button variant="ghost" size="sm" onClick={doRollback} disabled={busy || restarting}>
              <RotateCcwIcon className="size-4" />
              Zur vorherigen Version zurückkehren
            </Button>
          )}
        </div>

        <p className="text-xs text-muted-foreground">
          Ein-Klick-Aktualisierungen verlassen sich auf das Supervisor-Skript zum Neustart des Programms. Starten Sie ARTEX über
          <span className="font-mono">start.sh</span> (unter Windows, <span className="font-mono"> start.bat</span>);
          if you run the artex binary directly, the program will not be restarted automatically after it exits.
        </p>
      </CardContent>
    </Card>
  );
}
