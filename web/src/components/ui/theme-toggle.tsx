"use client";

import { useSyncExternalStore } from "react";
import { Button } from "./button";

type Theme = "light" | "dark";

const THEME_CHANGE_EVENT = "agenda-theme-change";

function readClientTheme(): Theme {
  if (typeof window === "undefined") {
    return "light";
  }
  const attr = document.documentElement.getAttribute("data-theme");
  if (attr === "dark" || attr === "light") {
    return attr;
  }
  const stored = window.localStorage.getItem("agenda-theme");
  if (stored === "dark" || stored === "light") {
    return stored;
  }
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

function subscribeTheme(onStoreChange: () => void): () => void {
  if (typeof window === "undefined") {
    return () => {};
  }
  const mql = window.matchMedia?.("(prefers-color-scheme: dark)");
  window.addEventListener(THEME_CHANGE_EVENT, onStoreChange);
  mql?.addEventListener?.("change", onStoreChange);
  return () => {
    window.removeEventListener(THEME_CHANGE_EVENT, onStoreChange);
    mql?.removeEventListener?.("change", onStoreChange);
  };
}

export function ThemeToggle() {
  const theme = useSyncExternalStore(
    subscribeTheme,
    readClientTheme,
    () => "light" as Theme,
  );

  function toggle() {
    const next: Theme = theme === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    window.localStorage.setItem("agenda-theme", next);
    window.dispatchEvent(new Event(THEME_CHANGE_EVENT));
  }

  return (
    <Button
      variant="secondary"
      size="sm"
      onClick={toggle}
      aria-pressed={theme === "dark"}
      aria-label={
        theme === "dark" ? "Cambiar a tema claro" : "Cambiar a tema oscuro"
      }
    >
      {theme === "dark" ? "Tema claro" : "Tema oscuro"}
    </Button>
  );
}
