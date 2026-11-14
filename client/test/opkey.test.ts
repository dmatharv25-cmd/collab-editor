import { describe, it, expect } from "vitest";
import { Doc, opKey } from "../src/crdt";

describe("opKey", () => {
  it("matches the server format for inserts and deletes", () => {
    const d = new Doc("A");
    const ins = d.localInsert(0, 120);
    expect(opKey(ins)).toBe("0|A:1|:0");
    const del = d.localDelete(0);
    expect(opKey(del)).toBe("1|:0|A:1");
  });
});
