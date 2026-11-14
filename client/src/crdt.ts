export interface ID {
  Client: string;
  Seq: number;
}

// Field names match Go's JSON encoding so ops pass over the wire unchanged.
export interface Op {
  Type: number; // 0 = insert, 1 = delete
  ID: ID;
  After: ID;
  Value: number; // Unicode code point
  Target: ID;
}

export const OP_INSERT = 0;
export const OP_DELETE = 1;

const ZERO: ID = { Client: "", Seq: 0 };
const isZero = (a: ID): boolean => a.Client === "" && a.Seq === 0;
const greater = (a: ID, b: ID): boolean =>
  a.Seq !== b.Seq ? a.Seq > b.Seq : a.Client > b.Client;
const key = (a: ID): string => `${a.Client}:${a.Seq}`;

class Item {
  deleted = false;
  prev: Item | null = null;
  next: Item | null = null;
  constructor(readonly id: ID, readonly value: number) {}
}

export class Doc {
  private clock = 0;
  private readonly head = new Item(ZERO, 0);
  private readonly index = new Map<string, Item>();
  private visible = 0;
  private fNode: Item;
  private fCount = 0;

  constructor(readonly client: string) {
    this.fNode = this.head;
  }

  get length(): number {
    return this.visible;
  }

  toString(): string {
    const parts: string[] = [];
    for (let n = this.head.next; n; n = n.next) {
      if (!n.deleted) parts.push(String.fromCodePoint(n.value));
    }
    return parts.join("");
  }

  private resetFinger(): void {
    this.fNode = this.head;
    this.fCount = 0;
  }

  private nodeAt(pos: number): Item {
    const target = pos + 1;
    let cur = this.fNode;
    let c = this.fCount;
    while (c < target) {
      cur = cur.next!;
      if (!cur.deleted) c++;
    }
    while (c > target || (c === target && cur.deleted)) {
      if (!cur.deleted) c--;
      cur = cur.prev!;
    }
    this.fNode = cur;
    this.fCount = c;
    return cur;
  }

  localInsert(pos: number, codePoint: number): Op {
    if (pos < 0 || pos > this.visible) throw new RangeError("position out of range");
    const after = pos > 0 ? this.nodeAt(pos - 1).id : ZERO;
    this.clock++;
    const op: Op = {
      Type: OP_INSERT,
      ID: { Client: this.client, Seq: this.clock },
      After: after,
      Value: codePoint,
      Target: ZERO,
    };
    const item = this.integrateInsert(op);
    if (typeof item === "string") throw new Error("local insert failed: " + item);
    this.fNode = item;
    this.fCount = pos + 1;
    return op;
  }

  localDelete(pos: number): Op {
    if (pos < 0 || pos >= this.visible) throw new RangeError("position out of range");
    const n = this.nodeAt(pos);
    n.deleted = true;
    this.visible--;
    this.fNode = n;
    this.fCount = pos;
    return { Type: OP_DELETE, ID: ZERO, After: ZERO, Value: 0, Target: n.id };
  }

  // Returns true if applied (or a harmless duplicate), false if a dependency is missing.
  apply(op: Op): boolean {
    if (op.Type === OP_INSERT) {
      const r = this.integrateInsert(op);
      if (r === "missing") return false;
      if (r !== "dup") this.resetFinger();
      return true;
    }
    const n = this.index.get(key(op.Target));
    if (!n) return false;
    if (!n.deleted) {
      n.deleted = true;
      this.visible--;
      this.resetFinger();
    }
    return true;
  }

  private integrateInsert(op: Op): Item | "dup" | "missing" {
    if (this.index.has(key(op.ID))) return "dup";
    let prev = this.head;
    if (!isZero(op.After)) {
      const p = this.index.get(key(op.After));
      if (!p) return "missing";
      prev = p;
    }
    while (prev.next && greater(prev.next.id, op.ID)) prev = prev.next;
    const n = new Item(op.ID, op.Value);
    n.prev = prev;
    n.next = prev.next;
    if (prev.next) prev.next.prev = n;
    prev.next = n;
    this.index.set(key(op.ID), n);
    this.visible++;
    if (op.ID.Seq > this.clock) this.clock = op.ID.Seq;
    return n;
  }
}

// Applies ops, retrying ones whose dependencies have not arrived.
// Returns how many ops could never be applied (0 means all succeeded).
export function applyAll(doc: Doc, ops: Op[]): number {
  let pending = ops;
  while (pending.length > 0) {
    const next: Op[] = [];
    for (const op of pending) {
      if (!doc.apply(op)) next.push(op);
    }
    if (next.length === pending.length) return next.length;
    pending = next;
  }
  return 0;
}

// Identifies an op on the wire. Must match opKey in server/hub/hub.go.
export const opKey = (op: Op): string =>
  `${op.Type}|${op.ID.Client}:${op.ID.Seq}|${op.Target.Client}:${op.Target.Seq}`;
