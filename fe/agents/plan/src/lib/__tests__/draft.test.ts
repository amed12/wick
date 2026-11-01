import { describe, expect, it } from "vitest";
import { draftFromJob } from "../api.js";
import type { GenJob } from "../types.js";

const job = (result: unknown): GenJob => ({ id: "j1", status: "done", result });

describe("draftFromJob", () => {
  it("accepts a generated draft", () => {
    const d = draftFromJob(
      job({
        title: "Kasir",
        summary: "ringkas",
        prd: "# Kasir",
        features: [{ title: "Auth", tasks: [{ title: "Login" }] }],
      }),
    );
    expect(d?.prd).toBe("# Kasir");
    expect(d?.features).toHaveLength(1);
  });

  it("rejects an empty result", () => {
    expect(draftFromJob(job(undefined))).toBeNull();
    expect(draftFromJob(job({ prd: "" }))).toBeNull();
  });

  it("normalises a missing features array", () => {
    expect(draftFromJob(job({ prd: "# X" }))?.features).toEqual([]);
  });
});
