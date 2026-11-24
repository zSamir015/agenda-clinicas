export const WEEKDAY_LABELS = [
  "Domingo",
  "Lunes",
  "Martes",
  "Miércoles",
  "Jueves",
  "Viernes",
  "Sábado",
] as const;

export function parseTimeToMinutes(hhmm: string): number | null {
  const match = /^(\d{2}):(\d{2})$/.exec(hhmm.trim());
  if (!match) {
    return null;
  }
  const hours = Number(match[1]);
  const minutes = Number(match[2]);
  if (hours < 0 || hours > 24 || minutes < 0 || minutes >= 60) {
    return null;
  }
  const total = hours * 60 + minutes;
  if (total > 1440 || total % 5 !== 0) {
    return null;
  }
  return total;
}

export function formatMinutesToTime(totalMinutes: number): string {
  const clamped = Math.max(0, Math.min(1440, totalMinutes));
  const hours = Math.floor(clamped / 60);
  const minutes = clamped % 60;
  return `${String(hours).padStart(2, "0")}:${String(minutes).padStart(2, "0")}`;
}

export function formatCentsUSD(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

/**
 * Convierte una fecha YYYY-MM-DD en America/Panama (UTC-5 fijo) al rango UTC [from, to).
 */
export function panamaDayRangeISO(dateStr: string): {
  from: string;
  to: string;
} | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(dateStr)) {
    return null;
  }
  const start = new Date(`${dateStr}T00:00:00-05:00`);
  if (Number.isNaN(start.getTime())) {
    return null;
  }
  const end = new Date(start.getTime() + 24 * 60 * 60 * 1000);
  return {
    from: start.toISOString(),
    to: end.toISOString(),
  };
}

export function formatPanamaDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return iso;
  }
  return new Intl.DateTimeFormat("es-PA", {
    timeZone: "America/Panama",
    dateStyle: "short",
    timeStyle: "short",
    hour12: false,
  }).format(d);
}

export function formatPanamaTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return iso;
  }
  return new Intl.DateTimeFormat("es-PA", {
    timeZone: "America/Panama",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(d);
}
