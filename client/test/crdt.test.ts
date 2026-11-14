import { describe, it, expect } from "vitest";
import { Doc, applyAll } from "../src/crdt";

const cp = (s: string): number => s.codePointAt(0)!;

describe("Doc", () => {
  it("types and deletes locally", () => {
    const d = new Doc("A");
    for (const ch of "hello") d.localInsert(d.length, cp(ch));
    expect(d.toString()).toBe("hello");
    d.localDelete(1);
    expect(d.toString()).toBe("hllo");
    expect(d.length).toBe(4);
  });

  it("converges on concurrent inserts at the same position", () => {
    const a = new Doc("A");
    const b = new Doc("B");
    const opA = a.localInsert(0, cp("x"));
    const opB = b.localInsert(0, cp("y"));
    a.apply(opB);
    b.apply(opA);
    expect(a.toString()).toBe(b.toString());
    expect(a.length).toBe(2);
  });

  it("ignores duplicate delivery", () => {
    const a = new Doc("A");
    const b = new Doc("B");
    const ops = [..."abc"].map((ch) => a.localInsert(a.length, cp(ch)));
    applyAll(b, ops);
    applyAll(b, ops);
    expect(b.toString()).toBe("abc");
  });

  it("handles emoji", () => {
    const d = new Doc("A");
    d.localInsert(0, cp("\u{1F600}"));
    d.localInsert(1, cp("x"));
    expect(d.toString()).toBe("\u{1F600}x");
  });
});
