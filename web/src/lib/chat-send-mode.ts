"use client";

import * as React from "react";

import { getLocalStorageValue, setLocalStorageValue } from "@/lib/local-storage.client";

// Send/newline key binding for the chat input. A purely front-end preference: it only
// lives in localStorage, is not persisted server-side, and does not sync with the
// account, so it must be reset on a different browser. See issue #39 — 0.3.2 changed
// Ctrl+Enter to send into Enter to send; this brings the old binding back as an option.
export type ChatSendMode = "enter" | "ctrl-enter";

export const CHAT_SEND_MODE_KEY = "artex_chat_send_mode";
export const DEFAULT_CHAT_SEND_MODE: ChatSendMode = "enter";

export const CHAT_SEND_MODE_OPTIONS: { value: ChatSendMode; label: string }[] = [
  { value: "enter", label: "Enter zum Senden, Shift+Enter für neue Zeile" },
  { value: "ctrl-enter", label: "Strg+Enter zum Senden, Enter für neue Zeile" },
];

function parseMode(raw: string | null): ChatSendMode {
  return raw === "ctrl-enter" || raw === "enter" ? raw : DEFAULT_CHAT_SEND_MODE;
}

// The set of subscribers within this tab. The localStorage "storage" event only fires in
// *other* tabs, so after changing this in settings on the current page we must emit to
// notify the input on the same page; otherwise it only takes effect after a refresh.
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  window.addEventListener("storage", listener);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", listener);
  };
}

// Returns a string literal, so Object.is compares by value and useSyncExternalStore
// will not get stuck in a loop.
function getSnapshot(): ChatSendMode {
  return parseMode(getLocalStorageValue(CHAT_SEND_MODE_KEY));
}

// The server has no localStorage, so render the default first and let getSnapshot
// correct it after hydration.
function getServerSnapshot(): ChatSendMode {
  return DEFAULT_CHAT_SEND_MODE;
}

export function useChatSendMode(): ChatSendMode {
  return React.useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}

export function setChatSendMode(mode: ChatSendMode) {
  setLocalStorageValue(CHAT_SEND_MODE_KEY, mode);
  for (const listener of listeners) listener();
}

// shouldSubmitOnKey decides whether a keypress should send.
// isComposing / keyCode 229 means an IME (e.g. Chinese) is still composing, so the key
// must pass through; otherwise pressing Enter to pick a candidate would send by mistake.
// enter mode only excludes Shift, matching 0.3.2 exactly so users who never change the
// setting keep the same feel. ctrl-enter mode accepts both Ctrl and Cmd (macOS).
export function shouldSubmitOnKey(e: React.KeyboardEvent, mode: ChatSendMode): boolean {
  if (e.key !== "Enter") return false;
  if (e.nativeEvent.isComposing || e.nativeEvent.keyCode === 229) return false;
  if (mode === "ctrl-enter") return e.ctrlKey || e.metaKey;
  return !e.shiftKey;
}
