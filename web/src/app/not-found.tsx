"use client";

import Link from "next/link";

import { Button } from "@/components/ui/button";

export default function NotFound() {
  return (
    <div className="flex h-dvh flex-col items-center justify-center space-y-2 text-center">
      <h1 className="font-semibold text-2xl">Seite nicht gefunden.</h1>
      <p className="text-muted-foreground">Die gesuchte Seite konnte nicht gefunden werden.</p>
      <Link prefetch={false} replace href="/function/tasks">
        <Button variant="outline">Zurück zur Startseite</Button>
      </Link>
    </div>
  );
}
