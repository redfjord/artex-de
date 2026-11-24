"use client";

import { type ReactNode, useEffect, useRef, useState } from "react";

import { ArrowUpIcon, MessageCircleQuestionIcon, SquareIcon, Trash2Icon, XIcon } from "lucide-react";

import { Markdown } from "@/components/markdown";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Drawer, DrawerContent, DrawerDescription, DrawerHeader, DrawerTitle } from "@/components/ui/drawer";
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupTextarea } from "@/components/ui/input-group";
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable";
import { Skeleton } from "@/components/ui/skeleton";
import { useIsMobile } from "@/hooks/use-mobile";
import type { SideQuestions } from "@/hooks/use-side-questions";
import { cn } from "@/lib/utils";

type ComposerLayout = "inline" | "stacked";

const preparationLabels = {
  preparing: "Kontext wird vorbereitet…",
  summarizing_history: "Frühere Nebenfragen werden zusammengefasst…",
  compressing_snapshot: "Nebenkontext-Kopie wird komprimiert…",
  retrying: "Modell-Kontext überschritten; wird reduziert und erneut versucht…",
  answering: "Wird beantwortet…",
};

export function SideQuestionButton({ side }: { side: SideQuestions }) {
  if (!side.enabled) return null;
  return (
    <Button variant="outline" size="sm" onClick={() => side.setOpen(true)} title="/btw Nebenfrage">
      <MessageCircleQuestionIcon data-icon="inline-start" />
      Nebenfrage
    </Button>
  );
}

function SidePanel({
  side,
  label,
  composerLayout,
}: {
  side: SideQuestions;
  label: string;
  composerLayout: ComposerLayout;
}) {
  const inlineComposer = composerLayout === "inline";
  const [confirm, setConfirm] = useState(false);
  const viewport = useRef<HTMLDivElement>(null);
  const pinned = useRef(true);
  const tail = side.items.at(-1);
  // biome-ignore lint/correctness/useExhaustiveDependencies: New cumulative text scrolls only readers who remain at the bottom.
  useEffect(() => {
    if (pinned.current && viewport.current) viewport.current.scrollTop = viewport.current.scrollHeight;
  }, [tail?.answer, tail?.id]);
  const status = { running: "Wird beantwortet", completed: "Abgeschlossen", failed: "Fehlgeschlagen", cancelled: "Gestoppt", interrupted: "Unterbrochen" };
  return (
    <section className="flex h-full min-h-0 flex-col bg-background" aria-label="Nebenfragen-Panel">
      <div className="flex items-center gap-2 border-b p-3">
        <div className="min-w-0 flex-1">
          <p className="font-medium">
            Nebenfrage <span className="text-muted-foreground">/btw</span>
          </p>
          <p className="truncate text-muted-foreground text-xs">{label}</p>
        </div>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => setConfirm(true)}
          disabled={!side.items.length || side.busy}
          aria-label="Nebenfragen-Verlauf löschen"
        >
          <Trash2Icon />
        </Button>
        <Button variant="ghost" size="icon-sm" onClick={() => side.setOpen(false)} aria-label="Nebenfragen-Panel schließen">
          <XIcon />
        </Button>
      </div>
      <div className="border-b px-3 py-2 text-muted-foreground text-xs">
        {side.snapshot ? (
          <>
            <p>{side.snapshot.model.model}</p>
            <p>Kontext aktualisiert am {new Date(side.snapshot.captured_at).toLocaleString()}</p>
          </>
        ) : (
          "Fragen sind verfügbar, nachdem der Haupt-Agent einmal ausgeführt wurde"
        )}
      </div>
      <div
        ref={viewport}
        onScroll={(event) => {
          const el = event.currentTarget;
          pinned.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
        }}
        className="min-h-0 flex-1 overflow-y-auto p-3"
      >
        {side.nextCursor > 0 && (
          <Button variant="ghost" size="sm" onClick={() => void side.load(side.nextCursor)}>
            Frühere Nebenfragen laden
          </Button>
        )}
        {side.loading && <Skeleton className="h-16 w-full" />}
        {!side.loading && side.items.length === 0 && (
          <Empty>
            <EmptyHeader>
              <EmptyTitle>Jederzeit eine Frage stellen</EmptyTitle>
              <EmptyDescription>Beantwortet aus dem aktuellen Kontext des Agents, während die Hauptaufgabe weiterläuft.</EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
        <div className="flex flex-col gap-5">
          {side.items.map((item) => (
            <article key={item.id} className="flex min-w-0 flex-col gap-2">
              <div className="whitespace-pre-wrap break-words rounded-lg bg-muted p-3 text-sm">{item.question}</div>
              <div className="flex flex-wrap items-center gap-2 text-muted-foreground text-xs">
                <Badge variant="secondary">{status[item.status]}</Badge>
                <span className="truncate">{item.model.model}</span>
                <time dateTime={item.snapshot_at} title={new Date(item.snapshot_at).toLocaleString()}>
                  Kontext {new Date(item.snapshot_at).toLocaleTimeString()}
                </time>
              </div>
              {item.context?.estimated_input_tokens != null && (
                <p className="text-muted-foreground text-xs">
                  Letzte {item.context.recent_exchanges} Q&A-Austausche wörtlich
                  {item.context.history_summarized && " · enthält frühere Q&A-Zusammenfassung"}
                  {item.context.snapshot_summarized && " · verwendet Hauptkontext-Zusammenfassung"}
                </p>
              )}
              {item.answer && <Markdown text={item.answer} />}
              {!item.answer && item.status === "running" && (
                <p role="status" className="text-muted-foreground text-sm">
                  {preparationLabels[item.context?.phase ?? "answering"]}
                </p>
              )}
              {item.error && (
                <Alert variant="destructive">
                  <AlertDescription>{item.error}</AlertDescription>
                </Alert>
              )}
            </article>
          ))}
        </div>
      </div>
      <div className="shrink-0 border-t p-3">
        {(side.error || side.snapshot?.reason) && (
          <Alert variant="destructive" className="mb-2">
            <AlertDescription>{side.error || side.snapshot?.reason}</AlertDescription>
          </Alert>
        )}
        <InputGroup className={inlineComposer ? "min-h-10" : "min-h-9"}>
          <InputGroupTextarea
            rows={1}
            className={cn("overflow-y-auto", inlineComposer ? "max-h-40 min-h-0" : "max-h-36 min-h-9")}
            aria-label="Nebenfrage"
            placeholder="Fragen Sie zum aktuellen Kontext…"
            value={side.draft}
            maxLength={4000}
            disabled={side.busy}
            onChange={(event) => side.setDraft(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
                event.preventDefault();
                void side.ask(side.draft);
              }
            }}
          />
          <InputGroupAddon align={inlineComposer ? "inline-end" : "block-end"}>
            {!inlineComposer && <span className="text-muted-foreground text-xs">Eigenständige Q&A · keine Tool-Ausführung</span>}
            {side.running ? (
              <InputGroupButton
                className="ml-auto"
                variant="destructive"
                size="icon-xs"
                onClick={() => void side.stop()}
                aria-label="Nebenfrage-Antwort stoppen"
              >
                <SquareIcon />
              </InputGroupButton>
            ) : (
              <InputGroupButton
                className="ml-auto"
                variant="default"
                size="icon-xs"
                onClick={() => void side.ask(side.draft)}
                disabled={side.busy || !side.draft.trim() || !side.snapshot?.available}
                aria-label="Nebenfrage senden"
              >
                <ArrowUpIcon />
              </InputGroupButton>
            )}
          </InputGroupAddon>
        </InputGroup>
      </div>
      {inlineComposer && (
        <div className="shrink-0 truncate px-3 pt-0.5 pb-1 text-muted-foreground text-xs">Eigenständige Q&A · keine Tool-Ausführung</div>
      )}
      <AlertDialog open={confirm} onOpenChange={setConfirm}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Nebenfragen-Verlauf löschen?</AlertDialogTitle>
            <AlertDialogDescription>
              Löscht die Nebenfragen dieses Agents und stoppt die Generierung laufender Antworten. Die Hauptkonversation und der Kontext-Snapshot bleiben erhalten.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Abbrechen</AlertDialogCancel>
            <AlertDialogAction onClick={() => void side.clear()}>Verlauf löschen</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}

export function SideQuestionWorkspace({
  side,
  label,
  children,
  composerLayout = "stacked",
}: {
  side: SideQuestions;
  label: string;
  children: ReactNode;
  composerLayout?: ComposerLayout;
}) {
  const mobile = useIsMobile();
  return (
    <>
      <ResizablePanelGroup orientation="horizontal" className="min-h-0 min-w-0 flex-1">
        <ResizablePanel id="main-conversation" minSize="35%" className="flex min-h-0 min-w-0 flex-col">
          {children}
        </ResizablePanel>
        {side.open && side.enabled && !mobile && (
          <>
            <ResizableHandle withHandle />
            <ResizablePanel id="side-question" defaultSize="38%" minSize="280px" maxSize="65%">
              <SidePanel side={side} label={label} composerLayout={composerLayout} />
            </ResizablePanel>
          </>
        )}
      </ResizablePanelGroup>
      <Drawer open={mobile && side.open && side.enabled} onOpenChange={side.setOpen}>
        <DrawerContent className="h-[85svh]">
          <DrawerHeader className="sr-only">
            <DrawerTitle>Nebenfrage</DrawerTitle>
            <DrawerDescription>Eigenständige Q&A für {label}</DrawerDescription>
          </DrawerHeader>
          <SidePanel side={side} label={label} composerLayout={composerLayout} />
        </DrawerContent>
      </Drawer>
    </>
  );
}
