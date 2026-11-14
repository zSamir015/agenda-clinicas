import { describe, expect, it } from "vitest";
import { createApiClient, fetchHealth } from "./client";

function fakeFetch(
  status: number,
  body: unknown,
  contentType = "application/json",
): typeof fetch {
  return async () =>
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": contentType },
    });
}

describe("fetchHealth", () => {
  it("returns ok when /readyz answers 200", async () => {
    const client = createApiClient(
      "http://api.test",
      fakeFetch(200, { status: "ok" }),
    );
    expect(await fetchHealth(client)).toBe("ok");
  });

  it("returns unavailable on 503", async () => {
    const client = createApiClient(
      "http://api.test",
      fakeFetch(
        503,
        { title: "x", status: 503, code: "database_unavailable" },
        "application/problem+json",
      ),
    );
    expect(await fetchHealth(client)).toBe("unavailable");
  });

  it("returns unavailable when the network fails", async () => {
    const failing: typeof fetch = async () => {
      throw new TypeError("fetch failed");
    };
    expect(await fetchHealth(createApiClient("http://api.test", failing))).toBe(
      "unavailable",
    );
  });
});
