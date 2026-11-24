# Sicherheitsnachweis ARTEX-DE

Der Go-Tokenvergleich mit FailSafe ARTEX `v0.3.15-safe` prüft auf veränderte Go-Logik.
Für die Weboberfläche zeigt [`verify/ui-compatibility.patch`](verify/ui-compatibility.patch)
die separat geprüften Funktionsänderungen. Der Syntaxbaumvergleich betrachtet danach den
reinen Übersetzungsstand.

Diese Behauptung ist nicht bloß versprochen, sondern **maschinell überprüfbar**. Alle Werkzeuge
und Eingaben liegen im Repository; jeder kann den Nachweis mit einem Befehl selbst wiederholen:

```bash
./verify/run-verification.sh
```

## Ergebnis auf einen Blick

| Prüfung | Ergebnis |
|---|---|
| Go-Code: Token-Vergleich aller 335 `.go`-Dateien | **PASS**: Code token-identisch; 149 Dateien mit geänderten Text-Literalen (1805 Literale), 186 ohne geänderte Text-Literale |
| Geänderte Go-Literale enthielten vorher Chinesisch | **PASS**: kein anderes Literal angefasst |
| Web-UI: Syntaxbaum-Vergleich aller 189 TS/TSX-Dateien | **PASS** nach Rücknahme des dokumentierten UI-Anpassungspatches. 92 Dateien mit geändertem Text, 97 ohne Textänderung |
| Neue Dateien / gelöschte Dateien im Code | **0 / 0** |
| Neue URLs oder Domains in Texten | **0** (Go und Web) |
| Abhängigkeits-, Build- und Laufzeitdateien | **byte-identisch** (`go.mod`, `go.sum`, `package.json`, `package-lock.json`, `Dockerfile.safe`, `docker-compose.safe.yml`, `start.sh`, `Makefile`, `.dockerignore`, `skills/`) |
| Indikatoren des `tomicdd`-Backdoors (`verifyCaptcha`, `tomicdd`, `congdonglinux`) | **nicht vorhanden** (Quelltext und gebautes Binary, siehe unten) |
| API-Schlüssel, Tokens und private Schlüssel in den geprüften Dateien | **0 Treffer** |
| Go-Testsuite (`go test ./...`) | Auf `main` mit isoliertem PostgreSQL: **676 bestanden, 0 fehlgeschlagen, 1 übersprungen** ([CI-Lauf](https://github.com/redfjord/artex-de/actions/runs/45385588981)). |

Die Rohausgaben stehen in `verify/RESULTS-go.txt`, `verify/RESULTS-web.txt` und
`verify/RESULTS-tests.txt`. Die letztere Datei dokumentiert den früheren Vergleich ohne PostgreSQL.
Der Tag `v0.3.15-de.1` und sein ZIP archivieren den Stand vor den Testkorrekturen.
Der [CI-Lauf des Tags](https://github.com/redfjord/artex-de/actions/runs/45378515675) meldet 19 Fehlschläge.

## Warum dieser Nachweis aussagekräftig ist

Schadcode, z. B. ein Datendieb (Stealer) oder eine Hintertür, braucht **ausführbaren Code**:
Funktionsaufrufe, neue Netzwerkziele, neue Abhängigkeiten oder neue Dateien. Genau das schließen
die Prüfungen aus:

- **Go-Tokenvergleich** (`verify/go-tokens/main.go`, ca. 270 Zeilen, nur Standardbibliothek): Jede
  Datei wird mit `go/scanner` zerlegt. Schlüsselwörter, Bezeichner, Operatoren, Zahlen und
  Satzzeichen müssen in derselben Reihenfolge vorkommen. Nur der *Inhalt* von Zeichenketten darf
  abweichen. Ein einziger zusätzlicher Funktionsaufruf oder eine veränderte Bedingung führt zu
  `FAIL`.
- **TypeScript-Syntaxbaum** (`verify/ts-ast-compare.mjs`, ca. 120 Zeilen): Jede Datei wird mit
  dem im Projekt-Lockfile festgelegten TypeScript-Compiler geparst. Der gesamte Baum, also Imports,
  Funktionsaufrufe, Bezeichner, JSX-Tags und Attributnamen, muss identisch sein. Nur Texte dürfen
  abweichen. JSX-Text wird von React automatisch escaped und kann keinen Code ausführen.
- **Text kann nicht heimlich zur Adresse werden:** Ein Angreifer könnte versuchen, in einer
  Zeichenkette eine Ziel-URL zu verstecken, etwa in einem `fetch("…")`. Beide Prüfer sammeln
  deshalb alle URLs und Domains aus allen Texten von Basis und Fork. Jede neue Adresse wäre ein
  `FAIL`. Es gibt keine.
- **Keine Lieferkettenänderung:** Lockfiles, Modul-Prüfsummen (`go.sum`), Dockerfile mit
  digest-gepinnten Basis-Images und Build-Skripte sind byte-identisch. Es wird also exakt
  dieselbe Software-Lieferkette gebaut wie bei FailSafe.

## Grenzen des Nachweises (ehrlich)

- Der Nachweis deckt die verglichenen Go- und Webdateien nach Rücknahme des UI-Anpassungspatches ab. Die Sicherheitsbewertung der Basis ist separat dokumentiert. ARTEX ist ein Angriffswerkzeug für
  autorisierte Tests und kann *bestimmungsgemäß* Befehle ausführen und Netzwerkverkehr erzeugen.
  Zur Sicherheit der Basis siehe FailSafes [Audit](audit/artex-v0.3.15-audit.md),
  [UPSTREAM.md](UPSTREAM.md) und [SECURITY.md](SECURITY.md).
- Agenten-Prompts sind Text und wurden übersetzt. Text führt keinen Code aus, beeinflusst aber das
  **Verhalten der KI-Agenten**. Die deutschen Prompts wurden geprüft, das Verhalten kann sich
  dennoch leicht vom Original unterscheiden.
- Die rekonstruierten Go- und Webdateien werden samt Pfaden gegen `verify/reference-sha256.txt` geprüft. Die Prüfsumme stammt aus der ursprünglichen Vergleichsbasis. Die Herkunft dieser Basis lässt sich zusätzlich mit `RuoJi6/ARTEX` Tag `v0.3.15`
  (`a91b27feb7168c1486f827559bc48a589a920012`) vergleichen. Die dabei sichtbaren Unterschiede sind
  in [UPSTREAM.md](UPSTREAM.md) einzeln dokumentiert.

## Herkunftskette

```
ARTEX (Autumn-27)
  └─ RuoJi6/ARTEX  Tag v0.3.15  (a91b27feb7168c1486f827559bc48a589a920012)
       └─ FailSafe ARTEX v0.3.15-safe   gehärtet & auditiert, dokumentiert in UPSTREAM.md / audit/
            └─ ARTEX-DE                 Übersetzung plus dokumentierte UI-Anpassungen
```

## Manuelle Prüfung der Übersetzung

Neben dem Code-Nachweis wurde geprüft, dass die Übersetzung die **Funktion** nicht verändert:

- **Texte an Logik-Stellen:** Die Prüfer listen jede geänderte Zeichenkette auf, die in einem
  Vergleich, einem `switch`, als Schlüssel, in `strings.HasPrefix/Contains` oder in SQL steht
  (Abschnitt `REVIEW` der Ergebnisdateien). Alle 15 Stellen im Produktionscode wurden einzeln
  geprüft. Es handelt sich ausschließlich um ausgegebene Werte, etwa Fehlertexte in
  `UPDATE … SET error=…`, Ersatz-Bezeichnungen in `COALESCE(…, 'nicht kategorisiert')` und
  Beschreibungen in `INSERT … VALUES`. Kein übersetzter Text steht in einer `WHERE`-Bedingung. Der
  Vergleich `c.Title == "Neue Unterhaltung"` verwendet denselben Text, mit dem neue Unterhaltungen
  angelegt werden.
- **Platzhalter-Reihenfolge:** Alle 214 übersetzten Go-Texte mit zwei oder mehr Platzhaltern
  (`%s`, `%d` …) und alle 29 Web-Templates mit zwei oder mehr Ausdrücken wurden auf vertauschte
  Bedeutung geprüft.
- **Testsuite:** Die ursprünglichen Tests laufen und bestätigen das Verhalten (siehe oben).

### Bei der Prüfung gefundene und behobene Übersetzungsfehler

Die automatische Erstübersetzung enthielt Fehler. Die Prüfwerkzeuge und die Testsuite haben sie
gefunden; alle sind behoben. Zur Transparenz die vollständige Liste:

| Fund | Auswirkung ohne Korrektur | Korrektur |
|---|---|---|
| KI-Freigaberichter: Prompt verlangte „Folge **bei** Erfolg", Parser suchte „Folge **nach** Erfolg" | Urteile unlesbar → Standard-Rückfall *erlauben* | `intercept/prompt.go` vollständig auf geprüftes Original zurückgesetzt |
| Guthaben-Fehlerbegriffe chinesischer LLM-Anbieter übersetzt | kein automatischer Modellwechsel bei leerem Kontingent | Originalbegriffe wiederhergestellt |
| ICP-Erkennungsbegriff `备案` übersetzt | chinesische ICP-Eingaben nicht erkannt | Original wiederhergestellt |
| Standard-Chattitel beim Anlegen ≠ Titel beim Vergleich | automatische Benennung neuer Chats blieb aus | beide auf „Neue Unterhaltung" |
| Platzhalter vertauscht („%d Ereignisse von Aufgabe #%d") | Aufgaben-ID und Anzahl vertauscht angezeigt | Wortstellung korrigiert |
| `request`/`response` als Logikwerte übersetzt (Web) | Build-Fehler (Typprüfung) | Logikwerte zurückgesetzt, nur Anzeige übersetzt |
| `data-slot="carousel"` übersetzt | CSS-Selektor greift nicht | Original wiederhergestellt |
| Kurztexte für Abbruchgründe länger als 40 Zeichen | Verletzung des UI-Limits | gekürzt |
| Mehrere Syntaxfehler durch die Rohübersetzung (Anführungszeichen, Backticks) | Build schlug fehl | behoben; Build, `go vet` und alle Prüfer bestehen |

## Selbst nachprüfen

Voraussetzungen: `git`, Go ≥ 1.22, Node.js ≥ 20 mit npm.

```bash
git clone <URL dieses Repositories> artex-de
cd artex-de
./verify/run-verification.sh
```

Das Skript arbeitet in vier Schritten:

1. Es kopiert den Quellbaum in ein temporäres Verzeichnis und nimmt dort den dokumentierten
   UI-Anpassungspatch zurück. Aus dieser Kopie rekonstruiert es per Übersetzungspatch die
   FailSafe-Basis und vergleicht ihre Go- und Webdateien mit der festgelegten Prüfsumme.
2. Es führt den Go-Tokenvergleich aus.
3. Es installiert per `npm ci --ignore-scripts` exakt die im Lockfile festgelegten Pakete und führt
   den TypeScript-Syntaxbaum-Vergleich auf dem reinen Übersetzungsstand aus.
4. Es sucht im Quelltext nach den Indikatoren des bekannten Backdoors.

Wer den Werkzeugen selbst nicht traut: Beide Prüfer sind kurz und nutzen nur die
Go-Standardbibliothek bzw. den TypeScript-Compiler. Alternativ zeigt
`git apply --stat verify/german-translation.patch` bzw. der Patch selbst jede einzelne geänderte
Zeile.

## Prüfsummen

SHA-256 der Nachweisdateien auf diesem Stand:

```
ab95c3aa1b9851c4951d9df3f0e9104387a5db8a755c7760c801b204397aa8da  verify/german-translation.patch
5753e1f9a21db1c6ec7118abc01ca789b00aff491148196bb215cc2bac332623  verify/go-tokens/main.go
456fd6f76f8e2d54146b16642149066d38c7603b3e528b77c660769bac06e2ae  verify/go-tokens/go.mod
27ea817f034e8178a262475872cfa3b3d8b49096829b6a4b88d1969259e3b79b  verify/ts-ast-compare.mjs
847577260f43b9efbd46002ff0c5a4e99cfe956362b173e4662b952925513c72  verify/run-verification.sh
3f3616e8fa606e5967b8f47729247548389901ae03db65540e26ef7bd64b8afc  verify/ui-compatibility.patch
e6fc1352d3563ae359dfef35a7f8c862869d586537dee291d3cc1fb97a0cbe6e  verify/reference-sha256.txt
c2659860c29a92a685d8bbdc45d02217f69ee08036fc818a7bd455f76f168704  verify/source-digest.mjs
```

### Gebautes Binary (Referenz-Build)

Gebaut mit `Dockerfile.safe` am 24.11.2026 auf `linux/arm64`. Die Prüfung entspricht FailSafes
`make verify`: Container ohne Netzwerk, `strings /app/artex`.

```
99e77f088bcccfdc4ffa5317290f4ed2d523a454f679b78206550fbc05fc28cb  /app/artex   (linux/arm64)
absent: verifyCaptcha
absent: tomicdd
absent: congdonglinux
```

Auf einer anderen Architektur, etwa `linux/amd64`, entsteht ein anderes Binary mit anderer
Prüfsumme. Die Indikator-Prüfung lässt sich nach dem eigenen Build jederzeit wiederholen:

```bash
make build && make verify
```

