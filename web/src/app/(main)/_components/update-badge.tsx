"use client";

import * as React from "react";

import Link from "next/link";

import { ArrowUpCircleIcon } from "lucide-react";

import { api } from "@/lib/api";

/**
 * Top-bar "new version available" notice: checks once on full page load and shows
 * up next to the version number when an update exists. Clicking it goes straight
 * to the "Version & updates" card on the system settings page.
 *
 * The backend caches GitHub queries for 30 minutes, so checking once per mount is safe
 * — the unauthenticated GitHub API allows only 60 requests/hour/IP; without that cache,
 * opening a few tabs would exhaust the quota and you couldn't check when you actually need to update.
 *
 * Query failures are always silent: the top bar isn't the place for errors; the user
 * will see the reason after opening settings and clicking "Check for updates".
 */
export function UpdateBadge() {
  const [latest, setLatest] = React.useState("");

  React.useEffect(() => {
    let alive = true;
    api
      .checkUpdate()
      .then((r) => {
        // has_update already includes the "version numbers are comparable" check; dev builds never show this notice.
        if (alive && r.has_update && r.latest) setLatest(r.latest.replace(/^v(?=\d)/, ""));
      })
      .catch(() => {
        // Silent: neither offline nor GitHub rate limiting should surface an error in the top bar.
      });
    return () => {
      alive = false;
    };
  }, []);

  if (!latest) return null;

  return (
    <Link
      href="/system/settings"
      title={`Neue Version ${latest} verfügbar, klicken zum Aktualisieren`}
      className="inline-flex items-center gap-1.5 rounded-full bg-primary px-2.5 py-1 font-medium text-primary-foreground text-xs transition-opacity hover:opacity-90"
    >
      {/* Breathing dot: the top bar is busy, so plain text gets overlooked; the animation makes it obvious at a glance. */}
      <span className="relative flex size-1.5">
        <span className="absolute inline-flex size-full animate-ping rounded-full bg-primary-foreground opacity-75" />
        <span className="relative inline-flex size-1.5 rounded-full bg-primary-foreground" />
      </span>
      <ArrowUpCircleIcon className="size-3.5" />
      <span className="hidden sm:inline">Neue Version {latest}</span>
      <span className="sm:hidden">Neue Version</span>
    </Link>
  );
}
