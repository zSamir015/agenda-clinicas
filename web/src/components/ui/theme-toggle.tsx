"use client";

import { useState } from "react";
import { Button } from "./button";

type Theme = "light" | "dark";

function readInitialTheme(): Theme {
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

export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(readInitialTheme);

  function toggle() {
    const next: Theme = theme === "dark" ? "light" : "dark";
    setTheme(next);
    document.documentElement.setAttribute("data-theme", next);
    window.localStorage.setItem("agenda-theme", next);
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
