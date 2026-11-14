import { describe, it, expect } from "vitest";
import { diffCodePoints, mapIndex, type Edit } from "../src/diff";

const applyEdit = (a: number[], e: Edit): number[] => [
  ...a.slice(0, e.start),
  ...e.insert,
  ...a.slice(e.start + e.remove),
];
const rnd = (n: number): number[] =>
  Array.from({ length: n }, () => 97 + Math.floor(Math.random() * 3));

describe("diffCodePoints", () => {
  it("round-trips 2000 random pairs", () => {
    for (let i = 0; i < 2000; i++) {
      const a = rnd(Math.floor(Math.random() * 12));
      const b = rnd(Math.floor(Math.random() * 12));
      expect(applyEdit(a, diffCodePoints(a, b))).toEqual(b);
    }
  });
});

describe("mapIndex", () => {
  it("keeps the cursor after text inserted before it", () => {
    expect(mapIndex("hello", "XXhello", 3)).toBe(5);
  });
  it("leaves the cursor alone for text inserted after it", () => {
    expect(mapIndex("hello", "helloXX", 2)).toBe(2);
  });
});
