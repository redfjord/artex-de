"use client";

import * as React from "react";

import { RotateCcwIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Spinner } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";
import { api } from "@/lib/api";
import type { FindingRetest } from "@/lib/types";

interface FindingRetestDialogProps {
  findingId: string;
  findingName?: string;
  onClose: () => void;
  onStarted?: (retest: FindingRetest) => void;
}

// Mounted only while open; notes are cleared on close. The list and detail views
// share the submit lock and error handling, and stay on the current page after starting.
export function FindingRetestDialog({ findingId, findingName, onClose, onStarted }: FindingRetestDialogProps) {
  const notesId = React.useId();
  const [notes, setNotes] = React.useState("");
  const [submitting, setSubmitting] = React.useState(false);
  const submitLock = React.useRef(false);

  async function start() {
    if (submitLock.current) return;
    submitLock.current = true;
    setSubmitting(true);
    try {
      const result = await api.startFindingRetest(findingId, notes.trim());
      onStarted?.(result.retest);
      onClose();
      toast.success(result.created ? "Retest gestartet, klicken Sie auf \"Retesting\", um die Sitzung anzuzeigen" : "Dieser Fund wird bereits retestet; Sie können die bestehende Sitzung anzeigen");
    } catch (e) {
      toast.error(`Fehler beim Starten des Retests: ${(e as Error).message}`);
    } finally {
      submitLock.current = false;
      setSubmitting(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !submitLock.current && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Fund #{findingId} retesten</DialogTitle>
          <DialogDescription className="break-words">
            {findingName ? <span className="mb-2 block">{findingName}</span> : null}
            Der Retest-Agent liest die ursprünglichen Beweise und Testbeschränkungen und führt eine gezielte
            Verifikation in einer separaten Sitzung durch. Wenn der Retest erfolgreich abgeschlossen wird und den Fix bestätigt,
            ändert sich der Fund-Status automatisch zu "Behoben"; andere Ergebnisse behalten den ursprünglichen Status bei.
          </DialogDescription>
        </DialogHeader>
        <FieldGroup>
          <Field data-disabled={submitting}>
            <FieldLabel htmlFor={notesId}>Zusätzliche Hinweise (optional)</FieldLabel>
            <Textarea
              id={notesId}
              value={notes}
              maxLength={4000}
              rows={4}
              disabled={submitting}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="Beispiel: den ursprünglichen Endpunkt mit dem ursprünglichen Testkonto prüfen; der Fix wurde in v2 veröffentlicht."
            />
            <FieldDescription>Optional die Fix-Version, Testbedingungen oder aktuelle Einschränkungen angeben.</FieldDescription>
          </Field>
        </FieldGroup>
        <DialogFooter>
          <Button variant="outline" disabled={submitting} onClick={onClose}>
            Abbrechen
          </Button>
          <Button disabled={submitting} onClick={() => void start()}>
            {submitting ? <Spinner data-icon="inline-start" /> : <RotateCcwIcon data-icon="inline-start" />}
            {submitting ? "Wird erstellt…" : "Retest starten"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
