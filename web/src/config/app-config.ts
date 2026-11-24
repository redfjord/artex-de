import packageJson from "../../package.json";

const currentYear = new Date().getFullYear();

export const APP_CONFIG = {
  name: "ARTEX",
  version: packageJson.version,
  copyright: `© ${currentYear}, ARTEX.`,
  meta: {
    title: "ARTEX — Autonome Penetrationstest-Konsole",
    description: "Konsole für ein LLM-gesteuertes autonomes Penetrationstest-System",
  },
};
