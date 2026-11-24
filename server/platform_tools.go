package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Autumn-27/artex/db"
	actool "github.com/Autumn-27/norma/tool"
)

// 平台操作工具(给内置 Auto agent 用):建/改 skill、自定义工具、MCP。都是 host 工具,
// seed 进 tools 表、默认绑定 auto,经 hostTools 注入。复用现有 db/文件系统逻辑。

func (s *Server) platformTools() []actool.CoreTool {
	return []actool.CoreTool{
		s.toolCreateSkill(),
		s.toolUpdateSkillFile(),
		s.toolCreateCustomTool(),
		s.toolUpdateCustomTool(),
		s.toolCreateMCP(),
		s.toolUpdateMCP(),
		s.toolDeleteAssetsByHost(),
	}
}

// platformToolKeys are the tool keys the Auto agent gets bound by default.
var platformToolKeys = []string{
	"create_skill", "update_skill_file",
	"create_custom_tool", "update_custom_tool",
	"create_mcp", "update_mcp",
	"delete_assets_by_host",
}

// ---- assets ----

// toolDeleteAssetsByHost hard-deletes every asset tied to one host (exact match).
// Platform-level (not a per-task tool): operates on the global, cross-task asset库.
func (s *Server) toolDeleteAssetsByHost() actool.CoreTool {
	return wrTool("delete_assets_by_host",
		"Nach host präzise Assets löschen: Domains/Subdomains des Hosts sowie dessen Services und Endpoints entfernen.\n"+
			"Host exakt matchen (Kleinbuchstaben, Leerzeichen entfernt), kein Fuzzy-/Wildcard-Matching.\n"+
			"Bei Root-Domain (z. B. example.com) werden auch deren Subdomains samt Services/Endpoints gelöscht; bei Subdomain (z. B. a.example.com) oder IP nur der Host selbst samt Services/Endpoints.\n"+
			"⚠️ Hard-Delete, betrifft globale Asset-Bibliothek (aufgabenübergreifend geteilt), nicht rückgängig machbar.",
		objSchema(map[string]any{
			"host": strParam("Zu löschender Host: Domain/Subdomain/IP. Exakte Übereinstimmung, z. B. example.com oder a.example.com oder 1.2.3.4"),
		}, "host"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			as := s.assetStore()
			if as == nil {
				return actool.Errorf("Asset-Bibliothek nicht initialisiert"), nil
			}
			var a struct {
				Host string `json:"host"`
			}
			_ = json.Unmarshal(in, &a)
			if strings.TrimSpace(a.Host) == "" {
				return actool.Errorf("Host darf nicht leer sein"), nil
			}
			counts, err := as.DeleteByHost(a.Host)
			if err != nil {
				return actool.Errorf("Löschen fehlgeschlagen: " + err.Error()), nil
			}
			var total int64
			for _, n := range counts {
				total += n
			}
			return jsonResult(map[string]any{
				"host":            a.Host,
				"deleted":         total,
				"deleted_by_type": counts,
			})
		})
}

// ---- skills ----

func (s *Server) toolCreateSkill() actool.CoreTool {
	return wrTool("create_skill",
		"Einen neuen Skill erstellen (SKILL.md schreiben, agentskills.io-Spezifikation). Name: Kleinbuchstaben/Ziffern/Bindestrich.",
		objSchema(map[string]any{
			"name":         strParam("Skill-Name (muss mit Kleinbuchstaben beginnen, Buchstaben/Ziffern/Bindestrich)"),
			"description":  strParam("Skill-Beschreibung (Pflichtfeld, erklärt Zweck/Verwendungszweck)"),
			"instructions": strParam("Markdown-Inhalt (optional)"),
		}, "name", "description"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct{ Name, Description, Instructions string }
			_ = json.Unmarshal(in, &a)
			if !validSkillName(a.Name) {
				return actool.Errorf("Skill-Name ungültig (muss mit Kleinbuchstaben beginnen, nur Buchstaben/Ziffern/Bindestrich, ≤64)"), nil
			}
			if strings.TrimSpace(a.Description) == "" {
				return actool.Errorf("description ist Pflichtfeld"), nil
			}
			path := filepath.Join(s.skillDir, a.Name)
			if _, err := os.Stat(path); err == nil {
				return actool.Errorf("Skill existiert bereits: " + a.Name), nil
			}
			if err := os.MkdirAll(path, 0o755); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			var b strings.Builder
			b.WriteString("---\n")
			fmt.Fprintf(&b, "name: %s\n", a.Name)
			fmt.Fprintf(&b, "description: %s\n", a.Description)
			b.WriteString("---\n")
			if strings.TrimSpace(a.Instructions) != "" {
				b.WriteString(a.Instructions)
			} else {
				fmt.Fprintf(&b, "## %s\n\n1. \n2. \n3. \n", a.Name)
			}
			if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(b.String()), 0o644); err != nil {
				_ = os.RemoveAll(path)
				return actool.Errorf(err.Error()), nil
			}
			return actool.Text("skill created: " + a.Name), nil
		})
}

func (s *Server) toolUpdateSkillFile() actool.CoreTool {
	return wrTool("update_skill_file",
		"Datei in einem Skill schreiben/überschreiben (Standard: SKILL.md). Dient zur Änderung von Skill-Inhalt oder Hinzufügen von Skripten/Referenzen.",
		objSchema(map[string]any{
			"name":    strParam("Skill-Name"),
			"file":    strParam("Relativer Pfad (optional, Standard SKILL.md, z. B. scripts/run.py)"),
			"content": strParam("Vollständiger Dateiinhalt"),
		}, "name", "content"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct{ Name, File, Content string }
			_ = json.Unmarshal(in, &a)
			if !validSkillName(a.Name) {
				return actool.Errorf("Skill-Name ungültig"), nil
			}
			skillPath := filepath.Join(s.skillDir, a.Name)
			if _, err := os.Stat(skillPath); os.IsNotExist(err) {
				return actool.Errorf("Skill nicht gefunden: " + a.Name), nil
			}
			rel := strings.TrimSpace(a.File)
			if rel == "" {
				rel = "SKILL.md"
			}
			clean, msg := skillRelPath(rel)
			if msg != "" {
				return actool.Errorf("Ungültiger Pfad: " + msg), nil
			}
			full := filepath.Join(skillPath, clean)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if err := os.WriteFile(full, []byte(a.Content), 0o644); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return actool.Text("skill file written: " + a.Name + "/" + clean), nil
		})
}

// ---- custom tools ----

type customToolToolInput struct {
	Key         string          `json:"key"`
	Description string          `json:"description"`
	Kind        string          `json:"kind"`
	Exec        json.RawMessage `json:"exec"`
	Schema      json.RawMessage `json:"schema"`
	Agents      []string        `json:"agents"`
	Deferred    bool            `json:"deferred"`
	Enabled     *bool           `json:"enabled"`
}

func customToolSchema(keyDesc string) map[string]any {
	return objSchema(map[string]any{
		"key":         strParam(keyDesc),
		"description": strParam("Beschreibung für das Modell"),
		"kind":        strParam("shell | command | script (nur Python) | http. shell=bash Umgebungsdeklaration (nur Modell mitteilen, dass Werkzeug direkt in bash aufrufbar, kein exec/schema nötig); die anderen drei benötigen exec"),
		"exec":        map[string]any{"type": "object", "description": "Ausführungsspezifikation (für shell nicht nötig): command→{command}; script→{code}; http→{method,url,headers,body,proxy,use_recording_proxy}"},
		"schema":      map[string]any{"type": "object", "description": "Parameter-JSON-Schema (für shell/command/script optional; für http Pflicht und muss properties enthalten)"},
		"agents":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Gebundene Agent-Keys (optional)"},
		"deferred":    map[string]any{"type": "boolean", "description": "Ob verzögert (für shell ungültig; nur für selten genutzte command/script/http-Werkzeuge aktivieren)"},
		"enabled":     map[string]any{"type": "boolean", "description": "Ob aktiviert (Standard true)"},
	}, "key", "kind")
}

func toDBTool(a customToolToolInput) *db.Tool {
	enabled := true
	if a.Enabled != nil {
		enabled = *a.Enabled
	}
	return &db.Tool{
		Key: a.Key, Description: a.Description, Schema: a.Schema, Agents: a.Agents,
		Enabled: enabled, Kind: a.Kind, Exec: a.Exec, Deferred: a.Deferred,
	}
}

func (s *Server) toolCreateCustomTool() actool.CoreTool {
	return wrTool("create_custom_tool", "【Wichtig】Wenn Werkzeuge installiert werden, die die Plattform nicht hat, rufen Sie dieses Werkzeug auf, um die installierten Werkzeuge in die Plattform aufzunehmen, damit die Plattform sie aufrufen kann! Erstellen Sie ein benutzerdefiniertes Werkzeug (shell/command/script/http). shell=bash Umgebungsdeklaration, nur key+description+agents erforderlich, kein exec/schema nötig.",
		customToolSchema("Werkzeug-Key (muss mit Kleinbuchstaben beginnen, nur Buchstaben/Ziffern/Unterstrich)"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a customToolToolInput
			_ = json.Unmarshal(in, &a)
			a.Key = strings.TrimSpace(a.Key)
			if !reToolKey.MatchString(a.Key) {
				return actool.Errorf("Key muss mit Kleinbuchstaben beginnen, nur Kleinbuchstaben/Ziffern/Unterstrich erlaubt"), nil
			}
			if a.Kind != "shell" && a.Kind != "command" && a.Kind != "script" && a.Kind != "http" {
				return actool.Errorf("kind muss shell / command / script / http sein"), nil
			}
			if a.Kind == "http" && !hasSchemaProps(a.Schema) {
				return actool.Errorf("http-Werkzeug muss Parameter-JSON-Schema bereitstellen (darf nicht leer sein)"), nil
			}
			if exist, _ := s.m.pg.GetTool(a.Key); exist != nil {
				return actool.Errorf("Key existiert bereits: " + a.Key), nil
			}
			if err := s.m.pg.CreateCustomTool(toDBTool(a)); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return actool.Text("custom tool created: " + a.Key), nil
		})
}

func (s *Server) toolUpdateCustomTool() actool.CoreTool {
	return wrTool("update_custom_tool", "Ein vorhandenes benutzerdefiniertes Werkzeug ändern (nach key).",
		customToolSchema("Key des zu ändernden benutzerdefinierten Werkzeugs"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a customToolToolInput
			_ = json.Unmarshal(in, &a)
			existing, _ := s.m.pg.GetTool(a.Key)
			if existing == nil || existing.System {
				return actool.Errorf("Nur benutzerdefinierte Werkzeuge änderbar: " + a.Key), nil
			}
			if a.Kind != "shell" && a.Kind != "command" && a.Kind != "script" && a.Kind != "http" {
				return actool.Errorf("kind muss shell / command / script / http sein"), nil
			}
			if a.Kind == "http" && !hasSchemaProps(a.Schema) {
				return actool.Errorf("http-Werkzeug muss Parameter-JSON-Schema bereitstellen (darf nicht leer sein)"), nil
			}
			if err := s.m.pg.UpdateCustomTool(toDBTool(a)); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return actool.Text("custom tool updated: " + a.Key), nil
		})
}

// ---- MCP ----

type mcpToolInput struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Transport string          `json:"transport"`
	Command   string          `json:"command"`
	Args      json.RawMessage `json:"args"`
	Env       json.RawMessage `json:"env"`
	URL       string          `json:"url"`
	Enabled   *bool           `json:"enabled"`
	Insecure  *bool           `json:"insecure"`
}

func mcpSchema(withID bool) map[string]any {
	props := map[string]any{
		"name":      strParam("MCP-Servername"),
		"transport": strParam("stdio | http / sse"),
		"command":   strParam("stdio-Startbefehl (z. B. npx)"),
		"args":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Befehlsparameter-Array"},
		"env":       map[string]any{"type": "object", "description": "Umgebungsvariablen {KEY:VALUE}"},
		"url":       strParam("http/sse-URL"),
		"enabled":   map[string]any{"type": "boolean", "description": "Ob aktiviert (Standard true)"},
		"insecure":  map[string]any{"type": "boolean", "description": "http: TLS-Zertifikatsprüfung überspringen (bei selbstsignierten Zertifikaten auf true setzen, Standard false)"},
	}
	required := []string{"name", "transport"}
	if withID {
		props["id"] = map[string]any{"type": "integer", "description": "ID des zu ändernden MCP-Servers"}
		required = []string{"id", "name", "transport"}
	}
	return objSchema(props, required...)
}

func (a mcpToolInput) toDB() *db.MCPServer {
	enabled := true
	if a.Enabled != nil {
		enabled = *a.Enabled
	}
	insecure := false
	if a.Insecure != nil {
		insecure = *a.Insecure
	}
	return &db.MCPServer{
		ID: a.ID, Name: a.Name, Transport: a.Transport, Command: a.Command,
		Args: a.Args, Env: a.Env, URL: a.URL, Enabled: enabled, Insecure: insecure,
	}
}

func (s *Server) toolCreateMCP() actool.CoreTool {
	return wrTool("create_mcp", "Einen MCP-Server erstellen (stdio/http/sse). Nach Erstellung müssen dessen Werkzeuge per Agent-Sichtbarkeit autorisiert werden.",
		mcpSchema(false),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a mcpToolInput
			_ = json.Unmarshal(in, &a)
			a.ID = 0
			if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Transport) == "" {
				return actool.Errorf("name / transport sind Pflichtfelder"), nil
			}
			id, err := s.m.pg.SaveMCP(a.toDB())
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return actool.Text(fmt.Sprintf("mcp created: id=%d name=%s", id, a.Name)), nil
		})
}

func (s *Server) toolUpdateMCP() actool.CoreTool {
	return wrTool("update_mcp", "Einen vorhandenen MCP-Server ändern (nach id).",
		mcpSchema(true),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a mcpToolInput
			_ = json.Unmarshal(in, &a)
			if a.ID == 0 {
				return actool.Errorf("id ist Pflichtfeld"), nil
			}
			if _, err := s.m.pg.SaveMCP(a.toDB()); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return actool.Text(fmt.Sprintf("mcp updated: id=%d", a.ID)), nil
		})
}
