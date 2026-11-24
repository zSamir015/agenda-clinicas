import { describe, expect, it } from "vitest";
import {
  formatCentsUSD,
  formatMinutesToTime,
  panamaDayRangeISO,
  parseTimeToMinutes,
} from "./schedule-utils";

describe("schedule-utils", () => {
  it("parses HH:MM multiples of 5 into minutes", () => {
    expect(parseTimeToMinutes("08:00")).toBe(480);
    expect(parseTimeToMinutes("17:35")).toBe(1055);
    expect(parseTimeToMinutes("24:00")).toBe(1440);
    expect(parseTimeToMinutes("08:03")).toBeNull();
    expect(parseTimeToMinutes("25:00")).toBeNull();
    expect(parseTimeToMinutes("invalid")).toBeNull();
  });

  it("formats minutes back to HH:MM", () => {
    expect(formatMinutesToTime(480)).toBe("08:00");
    expect(formatMinutesToTime(1055)).toBe("17:35");
  });

  it("formats cents as USD", () => {
    expect(formatCentsUSD(5000)).toBe("$50.00");
    expect(formatCentsUSD(1550)).toBe("$15.50");
  });

  it("computes America/Panama UTC day range", () => {
    expect(panamaDayRangeISO("2026-11-24")).toEqual({
      from: "2026-11-24T05:00:00.000Z",
      to: "2026-11-25T05:00:00.000Z",
    });
    expect(panamaDayRangeISO("bad")).toBeNull();
  });
});
