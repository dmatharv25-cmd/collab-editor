import { readFileSync } from "node:fs";
import { describe, it, expect } from "vitest";
import { Doc, applyAll, type Op } from "../src/crdt";

interface Vector {
  name: string;
  ops: Op[];
  expected: string;
}

const vectors = JSON.parse(
  readFileSync(new URL("../../conformance/vectors.json", import.meta.url), "utf8"),
) as Vector[];

describe("Go to TypeScript conformance", () => {
  it("has vectors", () => {
    expect(vectors.length).toBeGreaterThan(0);
  });
  for (const v of vectors) {
    it(`${v.name} (${v.ops.length} ops, shuffled)`, () => {
      const d = new Doc("TS");
      expect(applyAll(d, v.ops)).toBe(0);
      expect(d.toString()).toBe(v.expected);
    });
  }
});
