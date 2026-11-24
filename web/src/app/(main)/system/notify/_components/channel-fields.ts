// Channel field tables and config value parsing helpers.
//
// Split from the page because this is **data**, not a view: it describes which fields
// each channel has, which control each should use, and the two-way conversion between
// form text and config values (JSON). Keeping it in a separate file means adding a
// channel only touches this file; the page itself need not change.
// Display names and descriptions for channel kinds. They live on the frontend because
// they only affect copy; the backend need not know about them.
export const KIND_LABEL: Record<string, string> = {
  dingtalk: "DingTalk",
  feishu: "Feishu",
  wecom: "WeCom",
  webhook: "Generischer Webhook",
  telegram: "Telegram",
  email: "E-Mail",
};

// Config field definitions for each channel.
//
// We deliberately keep a frontend field table instead of having the backend deliver a
// schema: the backend only handles Validate (required/format), while the UI needs layout
// and control types — different concerns. The only coupling point is secret_keys: the
// backend tells us which fields should render as password inputs, because only the
// channel implementation knows which values count as credentials (the entire WeCom
// webhook is a credential, while DingTalk's is just one secret). When adding a channel, a
// missing entry here only leaves the form blank; it never fails silently (hasFields below reports it).
export type FieldKind = "text" | "password" | "number" | "select" | "textarea" | "switch" | "kv" | "list";
export interface FieldDef {
  key: string;
  label: string;
  kind: FieldKind;
  placeholder?: string;
  help?: string;
  options?: { value: string; label: string }[];
}
export const CHANNEL_FIELDS: Record<string, FieldDef[]> = {
  dingtalk: [
    {
      key: "webhook",
      label: "Webhook-URL",
      kind: "text",
      placeholder: "https://oapi.dingtalk.com/robot/send?access_token=...",
    },
    {
      key: "secret",
      label: "Signatur-Geheimnis",
      kind: "password",
      help: "Ausfüllen, wenn die Sicherheitseinstellung des Bots auf \"Signieren\" steht; leer lassen für \"Benutzerdefiniertes Schlüsselwort\" oder keine Sicherheitseinstellung",
    },
  ],
  feishu: [
    {
      key: "webhook",
      label: "Webhook-URL",
      kind: "text",
      placeholder: "https://open.feishu.cn/open-apis/bot/v2/hook/...",
    },
    { key: "secret", label: "Signatur-Verifikations-Geheimnis", kind: "password", help: "Ausfüllen, wenn der Bot \"Signaturverifikation\" aktiviert hat; sonst leer lassen" },
  ],
  wecom: [
    {
      key: "webhook",
      label: "Webhook-URL",
      kind: "text",
      placeholder: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...",
    },
  ],
  webhook: [
    { key: "url", label: "Ziel-URL", kind: "text", placeholder: "https://your-endpoint.example.com/hook" },
    {
      key: "method",
      label: "Anfragemethode",
      kind: "select",
      options: [
        { value: "POST", label: "POST (mit Body)" },
        { value: "PUT", label: "PUT (mit Body)" },
        { value: "PATCH", label: "PATCH (mit Body)" },
        { value: "GET", label: "GET (ohne Body)" },
      ],
    },
    { key: "headers", label: "Benutzerdefinierte Header", kind: "kv", help: "Eine KEY=VALUE-Zeile pro Eintrag, z. B. Authorization=Bearer xxx" },
    {
      key: "body_template",
      label: "Anfragerumpf-Vorlage",
      kind: "textarea",
      help:
        "Leave blank to use the built-in default template. Variables: {{.Title}} {{.Batch}} {{.Count}} {{.HomeURL}} {{.SentAt}}," +
        " plus .Name/.VulnClass/.Severity/.Summary/.Assets/.DetailURL/.StatusLabel under range .Items." +
        " For inserted strings use {{json .Xxx}} rather than {{.Xxx}}, otherwise quotes in titles break the JSON.",
    },
  ],
  telegram: [
    { key: "bot_token", label: "Bot-Token", kind: "password", placeholder: "123456:ABC-DEF..." },
    { key: "chat_id", label: "Chat-ID", kind: "text", placeholder: "-1001234567890" },
    {
      key: "base_url",
      label: "API-URL",
      kind: "text",
      placeholder: "https://api.telegram.org",
      help: "Leer lassen, um die offizielle URL zu verwenden; ausfüllen, wenn ein selbst gehosteter Bot-API-Proxy genutzt wird",
    },
  ],
  email: [
    { key: "host", label: "SMTP-Server", kind: "text", placeholder: "smtp.example.com" },
    {
      key: "port",
      label: "Port",
      kind: "number",
      placeholder: "587",
      help: "587 nutzt STARTTLS; für 465 \"Implizites TLS\" aktivieren",
    },
    { key: "username", label: "Benutzername", kind: "text" },
    { key: "password", label: "Passwort / App-Code", kind: "password" },
    { key: "from", label: "Absender", kind: "text", placeholder: "artex@example.com" },
    { key: "to", label: "Empfänger", kind: "list", help: "Mehrere Adressen durch Kommas trennen" },
    { key: "tls", label: "Implizites TLS", kind: "switch", help: "Für Port 465 aktivieren; für 587 deaktiviert lassen (STARTTLS ist automatisch)" },
  ],
};

export const SEVERITY_OPTIONS = [
  { value: "", label: "Beliebig" },
  { value: "low", label: "Niedrig und höher" },
  { value: "medium", label: "Mittel und höher" },
  { value: "high", label: "Hoch und höher" },
  { value: "critical", label: "Nur Kritisch" },
];

export type ChannelForm = {
  name: string;
  kind: string;
  mode: "realtime" | "digest";
  enabled: boolean;
  ratePerMin: string;
  config: Record<string, unknown>;
  minSeverity: string;
  includeText: string;
  excludeText: string;
  taskIDsText: string;
  assetIDsText: string;
  onStatusChange: boolean;
};

export const emptyForm = (kind: string): ChannelForm => ({
  name: "",
  kind,
  mode: "realtime",
  enabled: true,
  ratePerMin: "",
  config: {},
  minSeverity: "",
  includeText: "",
  excludeText: "",
  taskIDsText: "",
  assetIDsText: "",
  onStatusChange: false,
});

// parseKV parses a "KEY=VALUE per line" textarea.
export function parseKV(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const t = line.trim();
    if (!t) continue;
    const i = t.indexOf("=");
    if (i > 0) out[t.slice(0, i).trim()] = t.slice(i + 1).trim();
  }
  return out;
}
// parseIDs parses a comma/whitespace-separated list of ids.
export function parseIDs(text: string): number[] {
  return text
    .split(/[\s,，]+/)
    .map((s) => s.trim())
    .filter(Boolean)
    .map((s) => Number(s))
    .filter((n) => Number.isFinite(n) && n > 0);
}
// parseKeywords parses a line/comma-separated keyword list (vulnerability class names may contain spaces, so split on lines or commas).
export function parseKeywords(text: string): string[] {
  return text
    .split(/[\n,，]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}
