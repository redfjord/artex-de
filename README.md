# ARTEX-DE — Deutsche Lokalisierung von FailSafe ARTEX

> **Inoffizieller Community-Fork.** Dieses Repository ist eine deutsche Übersetzung von
> [FailSafe ARTEX](README.failsafe.md) `v0.3.15`. Es wird **nicht** von FailSafe Cyber Frontier Labs,
> von `RuoJi6/ARTEX` oder von den ursprünglichen ARTEX-Autoren betrieben, geprüft oder unterstützt.
>
> **Nur für autorisierte Sicherheitstests.** ARTEX steuert autonome Agenten, die Shell-Befehle
> ausführen und Netzwerkverkehr erzeugen. Setze es ausschließlich gegen Systeme ein, die dir gehören
> oder für die du eine ausdrückliche schriftliche Erlaubnis hast.

*English summary: unofficial German localization of FailSafe ARTEX v0.3.15. Only display text was
translated. The included verifiers prove that the Go code is token-identical and the web UI's syntax
trees are identical to the FailSafe base, with no new files, dependencies, URLs or domains
(see [SICHERHEITSNACHWEIS.md](SICHERHEITSNACHWEIS.md), runnable via `verify/run-verification.sh`).*

---

## Was ist ARTEX?

ARTEX ist ein Open-Source-Werkzeug für autonome, KI-gestützte Penetrationstests: Ein Ziel-Agent
zerlegt den Auftrag, ein Planer erkundet ereignisgesteuert, parallele Worker führen Absichten aus,
Funde werden mit HTTP-Nachweisen belegt, und über eine Konsole steuert ein Mensch den Lauf.
Details zu Funktionen und Architektur stehen in der unveränderten
[FailSafe-README](README.failsafe.md) und der [Original-README](README.upstream.md).

## Was ist in diesem Fork anders?

**Nur die Sprache.** Gegenüber FailSafe ARTEX `v0.3.15` wurden ausschließlich Texte übersetzt:

| Bereich | Übersetzt |
|---|---|
| Web-Konsole | alle sichtbaren Texte (Navigation, Buttons, Tabellen, Dialoge, Hinweise, Meldungen) Englisch → Deutsch |
| Backend (Go) | Fehlermeldungen, Benachrichtigungen (E-Mail, Telegram, DingTalk, Feishu, WeCom, Webhook), Berichte, Tool-Beschreibungen, Agenten-Prompts und Log-Texte Chinesisch → Deutsch |
| Tests | erwartete Ausgabetexte an die deutschen Texte angepasst |

**Unverändert (byte-identisch zur FailSafe-Basis):** `go.mod`, `go.sum`, `web/package.json`,
`web/package-lock.json`, `Dockerfile.safe`, `docker-compose.safe.yml`, `start.sh`, `Makefile`,
`.dockerignore` und das komplette `skills/`-Verzeichnis. Es gibt keine neuen Abhängigkeiten,
keine neuen Programmdateien und keine neuen Netzwerkziele.

**Bewusst nicht übersetzt** (weil diese Texte Logik oder Protokoll sind, nicht Anzeige):

- **KI-Freigaberichter** (`intercept/prompt.go`) bleibt im geprüften Originalzustand. Er bewertet
  Agenten-Befehle vor der Ausführung; bei einem unlesbaren Urteil fällt er standardmäßig auf
  *erlauben* zurück. Eine Formatänderung durch Übersetzung wäre daher ein Sicherheitsrisiko. Seine
  Begründungstexte in der Freigabeliste erscheinen deshalb weiter auf Chinesisch.
- Der Markierungs-Token `[模型]` (unterscheidet Modell- von Regel-Entscheidungen).
- Die Erwähnungs-Kennungen im Chat (`@[漏洞#…]`), die Frontend und Backend gemeinsam verwenden.
- Erkennungsbegriffe für Eingaben Dritter: chinesische ICP-Registrierungen (`备案`) und
  Guthaben-Fehlermeldungen chinesischer LLM-Anbieter (`余额不足` …).
- Die Chat-Befehle `意图 …` / `提示 …` im Regelmodus. Sie funktionieren unverändert, zusätzlich
  werden wie im Original `intent …` und `hint …` akzeptiert.

## Sicherheit: Woran du erkennst, dass hier nichts versteckt ist

Ein Übersetzungs-Fork ist ein naheliegendes Versteck für Schadcode. Deshalb liegt ein
**reproduzierbarer Nachweis** bei, den jeder selbst ausführen kann:

```bash
./verify/run-verification.sh
```

Das Skript rekonstruiert die FailSafe-Basis aus `verify/german-translation.patch` und prüft danach:

1. **Go:** Alle 335 `.go`-Dateien werden mit dem offiziellen Go-Tokenizer zerlegt. Ohne
   Kommentare und Text-Literale ist der Code **token-identisch**. Geändert wurden ausschließlich
   Literale, die vorher chinesischen Text enthielten.
2. **Web:** Alle 189 TypeScript/TSX-Dateien werden mit dem projekteigenen TypeScript-Compiler
   geparst. Die **Syntaxbäume sind identisch**, nur Anzeigetexte unterscheiden sich.
3. **Keine neuen URLs oder Domains**, die die Prüfer in den untersuchten Go- und Web-Texten erkennen.
4. **Keine Indikatoren** des bekannten `tomicdd`-Backdoors (`verifyCaptcha` usw.).

Ergebnisse, Methodik, Grenzen des Nachweises und alle bei der Prüfung gefundenen und behobenen
Übersetzungsfehler: **[SICHERHEITSNACHWEIS.md](SICHERHEITSNACHWEIS.md)**.

## Schnellstart

Voraussetzungen: Docker Engine mit Compose v2 und etwa 5 GB freier Speicher. Falls nur das
eigenständige `docker-compose` installiert ist, ersetze `docker compose` entsprechend.

```bash
cp .env.safe.example .env.safe
chmod 600 .env.safe
# .env.safe bearbeiten: POSTGRES_PASSWORD setzen, optional einen LLM-Schlüssel
docker compose --env-file .env.safe -f docker-compose.safe.yml up -d --build
```

Danach <http://127.0.0.1:8787> öffnen und die Ersteinrichtung abschließen. Ohne LLM-Schlüssel in
`.env.safe` richtest du den Anbieter in der Konsole unter **System → LLM** ein.

```bash
# Status und Logs
docker compose --env-file .env.safe -f docker-compose.safe.yml ps
docker compose --env-file .env.safe -f docker-compose.safe.yml logs -f artex
# Stoppen, ohne Daten zu löschen
docker compose --env-file .env.safe -f docker-compose.safe.yml down
```

Die Konsole lauscht nur auf `127.0.0.1`, der Aufzeichnungs-Proxy wird nicht veröffentlicht, und
der automatische Updater ist im Quelltext hart deaktiviert (`selfupdate.Enabled = false`).
Updates erfolgen nur über einen geprüften Neubau aus dem Quelltext.

**`.env.safe` gehört niemals in ein Repository.** Sie ist in `.gitignore` eingetragen.

## Hinweis für bestehende Installationen

Einige Inhalte legt ARTEX beim **ersten** Start in der Datenbank ab, zum Beispiel Agenten-Prompts,
Namen eingebauter Agenten und Standard-Regeln. Wer von der chinesischen oder englischen Version
umsteigt und seine Datenbank behält, sieht diese gespeicherten Inhalte zunächst weiter in der alten
Sprache. Über **System → Agenten → Agent öffnen → „Standard wiederherstellen"** lädt man die
deutschen Standard-Prompts. Neue Installationen sind von Anfang an deutsch.

## Herkunft und Lizenz

| Stufe | Quelle |
|---|---|
| Original | ARTEX von Autumn-27 und Mitwirkenden |
| Quellbasis | [`RuoJi6/ARTEX`](https://github.com/RuoJi6/ARTEX), Tag `v0.3.15`, Commit `a91b27feb7168c1486f827559bc48a589a920012` |
| Gehärteter Build | FailSafe ARTEX `v0.3.15-safe`, siehe [UPSTREAM.md](UPSTREAM.md), [SECURITY.md](SECURITY.md) und [audit/](audit/) |
| Dieser Fork | deutsche Lokalisierung, siehe [SICHERHEITSNACHWEIS.md](SICHERHEITSNACHWEIS.md) |

Lizenz: **GNU Affero General Public License v3.0**, siehe [LICENSE](LICENSE).

**Änderungshinweis gemäß AGPL-3.0 §5a:** Dieses Werk wurde am **24.11.2026** gegenüber FailSafe
ARTEX `v0.3.15-safe` verändert: Übersetzung von Anzeige-, Meldungs- und Prompt-Texten ins Deutsche
und Anpassung der zugehörigen Test-Erwartungen. Die vollständige Liste aller Änderungen ist
`verify/german-translation.patch`. Hinzugefügt wurden nur Dokumentation (`README.md`,
`SICHERHEITSNACHWEIS.md`) und Prüfwerkzeuge (`verify/`). Die bisherige README liegt unverändert als
`README.failsafe.md` vor.

## Haftungsausschluss

Die Software wird ohne jede Gewährleistung bereitgestellt (siehe Lizenz). Du bist allein dafür
verantwortlich, dass jeder Einsatz rechtmäßig und autorisiert ist.
