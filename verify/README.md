# verify/ — Nachweis „nur Text übersetzt"

| Datei | Zweck |
|---|---|
| `run-verification.sh` | Führt den gesamten Nachweis aus (Basis rekonstruieren → Go-Prüfung → Web-Prüfung → Indikator-Suche). |
| `german-translation.patch` | Vollständiger Diff FailSafe ARTEX `v0.3.15-safe` → ARTEX-DE. Rückwärts angewendet ergibt er die Basis. |
| `go-tokens/` | Go-Prüfer: vergleicht die Token-Folge jeder `.go`-Datei ohne Kommentare und Text-Literale (nur Standardbibliothek). |
| `ts-ast-compare.mjs` | Web-Prüfer: vergleicht den TypeScript-Syntaxbaum jeder Datei ohne Anzeigetexte (nutzt das TypeScript aus `web/package-lock.json`). |
| `RESULTS-go.txt`, `RESULTS-web.txt` | Ausgaben der Prüfer für dieses Release (inkl. `REVIEW`-Liste der Texte an Logik-Stellen). |
| `RESULTS-tests.txt` | Vergleich `go test ./...` Basis vs. ARTEX-DE. |

Ein Prüfer meldet `FAIL`, wenn sich Code ändert, Dateien hinzukommen oder fehlen, oder irgendwo
eine neue URL bzw. Domain auftaucht. Details siehe [../SICHERHEITSNACHWEIS.md](../SICHERHEITSNACHWEIS.md).

Diese Prüfwerkzeuge sind nicht Teil der Anwendung. Sie werden weder gebaut noch ausgeliefert: Das
Docker-Image baut nur `./cmd/artex`, und `go-tokens` ist ein eigenes Go-Modul.
