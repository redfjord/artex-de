"use client";

import { useEffect, useRef, useState } from "react";

import { useRouter } from "next/navigation";

import { AlertTriangle, ShieldCheck } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";
import { auth } from "@/lib/auth";

export default function LoginPage() {
  const router = useRouter();
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(true);
  const [agreed, setAgreed] = useState(false);
  const [termsOpen, setTermsOpen] = useState(false);
  const [readToEnd, setReadToEnd] = useState(false);
  const termsBodyRef = useRef<HTMLDivElement>(null);

  // The "Agree" button is enabled only after scrolling to the bottom of the terms (including when everything fits without scrolling).
  function handleTermsScroll() {
    const el = termsBodyRef.current;
    if (!el) return;
    if (el.scrollTop + el.clientHeight >= el.scrollHeight - 8) setReadToEnd(true);
  }

  useEffect(() => {
    if (!termsOpen) return;
    // Reset when opened, and handle the case where the content is already shorter than one screen and cannot scroll.
    setReadToEnd(false);
    const el = termsBodyRef.current;
    if (el && el.scrollHeight <= el.clientHeight + 8) setReadToEnd(true);
  }, [termsOpen]);

  useEffect(() => {
    // If already logged in, go straight to the main UI (static export has no middleware to do this redirect).
    const token = auth.getToken();
    if (token) {
      // localStorage may still hold credentials while the cookie is gone. Sync first, then issue a fresh request,
      // so the server guard or route cache doesn't bounce the redirect back to a login page still in the checking state.
      auth.setToken(token);
      window.location.replace("/function/tasks");
      return;
    }
    api
      .authStatus()
      .then(({ initialized }) => {
        if (!initialized) router.replace("/setup");
      })
      .catch(() => setError("Keine Verbindung zum Backend-Dienst möglich"))
      .finally(() => setChecking(false));
  }, [router]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!agreed) {
      setError("Bitte lesen Sie zuerst die Nutzungsbedingungen und stimmen Sie zu");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const { token } = await api.login("ARTEX", password);
      auth.setToken(token);
      window.location.replace("/function/tasks");
    } catch {
      setError("Falscher Benutzername oder Passwort");
    } finally {
      setLoading(false);
    }
  }

  if (checking) {
    return (
      <div role="status" className="flex min-h-dvh items-center justify-center text-muted-foreground">
        Prüfe Anmeldestatus…
      </div>
    );
  }

  return (
    <div className="flex h-dvh">
      {/* Left panel */}
      <div className="hidden flex-col items-center justify-center bg-primary p-12 text-center lg:flex lg:w-1/3">
        <div className="relative flex items-center justify-center">
          <div className="absolute size-80 rounded-full border border-primary-foreground/10" />
          <div className="absolute size-60 rounded-full border border-primary-foreground/15" />
          <div className="absolute size-40 rounded-full border border-primary-foreground/20" />
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="/logo.png" alt="ARTEX" width={160} height={160} className="relative brightness-0 invert" />
        </div>
      </div>

      {/* Right panel */}
      <div className="flex w-full items-center justify-center bg-background p-8 lg:w-2/3">
        <div className="w-full max-w-md space-y-10 py-24 lg:py-32">
          <div className="space-y-4 text-center">
            <h2 className="text-2xl font-medium tracking-tight">Anmelden</h2>
            <p className="mx-auto max-w-xl text-muted-foreground">Willkommen zurück. Geben Sie Ihr Passwort ein, um ARTEX weiterzuverwenden.</p>
          </div>
          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            <div className="space-y-1.5">
              <Label htmlFor="username">Benutzername</Label>
              <Input id="username" value="ARTEX" readOnly className="bg-muted text-muted-foreground" />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="password">Passwort</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Passwort eingeben"
                autoFocus
                autoComplete="current-password"
              />
            </div>
            <div className="flex items-start gap-2">
              <Checkbox
                id="agree-terms"
                checked={agreed}
                onCheckedChange={(v) => setAgreed(v === true)}
                className="mt-0.5"
              />
              <Label htmlFor="agree-terms" className="text-sm font-normal leading-relaxed text-muted-foreground">
                Ich habe gelesen und stimme zu
                <button
                  type="button"
                  onClick={() => setTermsOpen(true)}
                  className="mx-0.5 font-medium text-primary underline-offset-4 hover:underline"
                >
                  Nutzungsbedingungen
                </button>
              </Label>
            </div>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <Button type="submit" className="w-full" disabled={loading || !password || !agreed}>
              {loading ? "Anmeldung läuft..." : "Anmelden"}
            </Button>
          </form>
        </div>
      </div>

      <Dialog open={termsOpen} onOpenChange={setTermsOpen}>
        <DialogContent className="gap-0 p-0 sm:max-w-2xl">
          <DialogHeader className="flex-row items-center gap-3 border-b px-6 py-4">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <ShieldCheck className="size-5" />
            </div>
            <div className="space-y-0.5">
              <DialogTitle className="text-base">ARTEX Nutzungsbedingungen und Haftungsausschluss</DialogTitle>
              <p className="text-xs text-muted-foreground">
                Version v1.0 · Gültig ab 2026-09-18 · Bitte lesen Sie alle Bedingungen unten vollständig, bevor Sie sich anmelden
              </p>
            </div>
          </DialogHeader>

          <div
            ref={termsBodyRef}
            onScroll={handleTermsScroll}
            className="max-h-[60vh] space-y-5 overflow-y-auto px-6 py-5 text-sm leading-relaxed text-muted-foreground"
          >
            <p className="rounded-lg border bg-muted/40 p-3 text-foreground/80">
              Diese Nutzungsbedingungen und Haftungsausschluss (die "Bedingungen") stellen die Vereinbarung zwischen Ihnen und ARTEX dar
              Projektautoren und Mitwirkende bezüglich der Nutzung dieser Software. Bitte lesen Sie sorgfältig und verstehen Sie jede Bestimmung vollständig, bevor Sie die Software verwenden, insbesondere die Haftungsausschlüsse, Haftungsbeschränkungen und Verbote, die fett oder in hervorgehobenen Blöcken markiert sind.
              <span className="font-medium text-foreground">
                {" "}
                Durch das Herunterladen, Installieren, Zugreifen oder Verwenden dieser Software in irgendeiner Weise gelten Sie als gelesen, verstanden und einverstanden, an alle diese Bedingungen gebunden zu sein.
              </span>
            </p>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  1
                </span>
                Artikel 1 · Definitionen und Open-Source-Lizenz
              </h4>
              <p className="pl-7">
                Diese Software (ARTEX) ist ein Open-Source-Programm, das unter der GNU Affero General Public License veröffentlicht wurde
                v3.0 (AGPL-3.0). Sie dürfen diese Software unter dieser Lizenz frei verwenden, kopieren, modifizieren und verbreiten; jedoch muss jede abgeleitete Arbeit (einschließlich Online-Dienste, die Dritten über ein Netzwerk bereitgestellt werden) ebenfalls
                unter der AGPL-3.0 als Open Source veröffentlicht werden und der vollständige entsprechende Quellcode den Nutzern zur Verfügung gestellt werden. Die vollständigen AGPL-3.0-Bedingungen unterliegen der beiliegenden LICENSE-Datei.
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  2
                </span>
                Artikel 2 · Zulässiger Nutzungsbereich
              </h4>
              <p className="pl-7">
                Diese Software ist ausschließlich für persönliches Lernen, Code-Studium, Diskussion von Sicherheitsprinzipien und technische Verifikation in einer lokalen isolierten Umgebung gedacht, die Sie selbst einrichten; sie eignet sich für nicht-offensive, nicht-destruktive Zwecke wie Lernen, akademische Forschung und Code-Review. Außer wie in diesem Artikel ausdrücklich erlaubt, dürfen Sie diese Software für keinen anderen Zweck verwenden.
              </p>
            </section>

            <section className="space-y-2">
              <h4 className="flex items-center gap-2 font-medium text-destructive">
                <span className="flex size-5 items-center justify-center rounded-md bg-destructive/10 text-xs font-semibold text-destructive">
                  3
                </span>
                <AlertTriangle className="size-4" />
                Artikel 3 · Verbotenes Verhalten
              </h4>
              <ul className="ml-7 list-decimal space-y-1.5 rounded-lg border border-destructive/20 bg-destructive/5 p-3 pl-8 text-foreground/80 marker:text-destructive/70">
                <li>
                  Es ist strengstens untersagt, Websites, Online-Dienste oder vernetzte Systeme, die anderen oder Dritten gehören, zu scannen, zu sondieren, auszunutzen oder anzugreifen (unabhängig davon, ob autorisiert oder ob es sich um Ihr eigenes Asset handelt);
                </li>
                <li>Es ist strengstens untersagt, diese Software für reale Penetrationstests, offensive/defensive Übungen, Red/Blue-Team-Übungen oder Produktionsumgebungen zu verwenden;</li>
                <li>Es ist strengstens untersagt, diese Software für unbefugtes Eindringen, Datendiebstahl, Erpressung, Denial-of-Service (DoS/DDoS) oder jede destruktive oder kriminelle Aktivität zu verwenden;</li>
                <li>Es ist strengstens untersagt, Urheberrechts-, Lizenz- oder Sicherheitshinweise in dieser Software oder ihrer Ausgabe zu entfernen, zu ändern oder zu umgehen;</li>
                <li>Es ist strengstens untersagt, sich an Verhaltensweisen zu beteiligen, die gegen die Gesetze, Vorschriften oder regulatorischen Anforderungen Ihres Landes oder Ihrer Region verstoßen.</li>
              </ul>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  4
                </span>
                Artikel 4 · Geistiges Eigentum
              </h4>
              <p className="pl-7">
                Das Urheberrecht und die damit verbundenen geistigen Eigentumsrechte an dieser Software gehören den Projektautoren und Mitwirkenden, und die entsprechenden Rechte werden Ihnen im Rahmen der AGPL-3.0 gewährt
                Lizenz. Außer den durch diese Lizenz ausdrücklich gewährten Rechten gewähren Ihnen diese Bedingungen keine weiteren Rechte, weder ausdrücklich noch stillschweigend.
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  5
                </span>
                Artikel 5 · Daten und Datenschutz
              </h4>
              <p className="pl-7">
                Diese Software ist ein Open-Source-Programm, das Sie selbst bereitstellen; die Autoren betreiben keinen zentralisierten Dienst und sammeln oder laden keine Nutzungsdaten hoch. Alle Daten, die Sie bei der Nutzung der Software erzeugen, verarbeiten oder auf die Sie zugreifen, liegen in Ihrer eigenen Kontrolle, und Sie sind für deren Rechtmäßigkeit und Sicherheit verantwortlich; Sie tragen die alleinige Verantwortung für alle Folgen, die aus unsachgemäßer Datenverarbeitung entstehen.
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  6
                </span>
                Artikel 6 · Compliance und rechtliche Haftung
              </h4>
              <p className="pl-7">
                Sie müssen alle Gesetze und Vorschriften Ihres Landes oder Ihrer Region in Bezug auf Cybersicherheit, Datensicherheit und Schutz personenbezogener Daten, Computerkriminalität und verwandte Angelegenheiten einhalten (in Festlandchina einschließlich, aber nicht beschränkt auf das Cybersicherheitsgesetz, Datensicherheitsgesetz, Gesetz zum Schutz personenbezogener Daten und relevante gerichtliche Auslegungen).
                <span className="font-medium text-foreground">
                  {" "}
                  Alle rechtliche Haftung und Konsequenzen, die aus Ihrem Verstoß gegen die oben genannten Gesetze und Vorschriften oder diese Bedingungen entstehen, tragen Sie allein und stehen in keinem Zusammenhang mit den Autoren und Mitwirkenden der Software.
                </span>
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  7
                </span>
                Artikel 7 · Haftungsausschluss und Haftungsbeschränkung
              </h4>
              <p className="pl-7">
                Diese Software wird "WIE BESEHEN" und "WIE
                VERFÜGBAR" bereitgestellt, ohne jegliche ausdrückliche oder stillschweigende Garantie, einschließlich, aber nicht beschränkt auf Garantien der Marktgängigkeit, Eignung für einen bestimmten Zweck, Richtigkeit und Nichtverletzung von Rechten. Soweit nach geltendem Recht zulässig, haften die Autoren und Mitwirkenden der Software nicht für direkte, indirekte, zufällige, besondere oder Folgeschäden, die aus der Nutzung oder Unmöglichkeit der Nutzung dieser Software entstehen (unabhängig davon, ob die Nutzung ordnungsgemäß war), einschließlich, aber nicht beschränkt auf Datenverlust, Systemschäden, Betriebsunterbrechung, entgangenen Gewinn oder Rechtsstreitigkeiten.
              </p>
            </section>

            <section className="space-y-1.5">
              <h4 className="flex items-center gap-2 font-medium text-foreground">
                <span className="flex size-5 items-center justify-center rounded-md bg-muted text-xs font-semibold text-muted-foreground">
                  8
                </span>
                Artikel 8 · Änderungen der Bedingungen und abschließende Auslegung
              </h4>
              <p className="pl-7">
                Die Autoren können diese Bedingungen von Zeit zu Zeit aktualisieren, soweit dies durch Gesetze, Vorschriften oder die Projektentwicklung erforderlich ist. Aktualisierte Versionen werden mit dem Projekt veröffentlicht und treten mit der Veröffentlichung in Kraft; Ihre fortgesetzte Nutzung der Software gilt als Annahme der geänderten Bedingungen. Soweit gesetzlich zulässig, behalten sich die Projektautoren das Recht der abschließenden Auslegung dieser Bedingungen vor. Sollte eine Bestimmung dieser Bedingungen für ungültig erklärt werden, bleibt die Gültigkeit der übrigen Bestimmungen unberührt.
              </p>
            </section>
          </div>

          <DialogFooter className="mx-0 mb-0 flex-col items-stretch gap-2 rounded-b-xl px-6 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-xs text-muted-foreground">
              {readToEnd ? "Sie haben alle Bedingungen geprüft" : "Scrollen Sie bis zum Ende der Bedingungen, um zu bestätigen"}
            </p>
            <DialogClose asChild>
              <Button
                type="button"
                disabled={!readToEnd}
                onClick={() => {
                  setAgreed(true);
                  setError("");
                }}
              >
                Ich habe alle Bedingungen gelesen und stimme zu
              </Button>
            </DialogClose>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
