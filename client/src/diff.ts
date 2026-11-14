export interface Edit {
  start: number;
  remove: number;
  insert: number[];
}

// Smallest single edit turning a into b (arrays of Unicode code points).
export function diffCodePoints(a: number[], b: number[]): Edit {
  const max = Math.min(a.length, b.length);
  let p = 0;
  while (p < max && a[p] === b[p]) p++;
  let s = 0;
  while (s < max - p && a[a.length - 1 - s] === b[b.length - 1 - s]) s++;
  return { start: p, remove: a.length - p - s, insert: b.slice(p, b.length - s) };
}

// Maps a cursor index in oldStr to the matching index in newStr.
export function mapIndex(oldStr: string, newStr: string, idx: number): number {
  const max = Math.min(oldStr.length, newStr.length);
  let p = 0;
  while (p < max && oldStr[p] === newStr[p]) p++;
  let s = 0;
  while (s < max - p && oldStr[oldStr.length - 1 - s] === newStr[newStr.length - 1 - s]) s++;
  if (idx <= p) return idx;
  if (idx >= oldStr.length - s) return idx + (newStr.length - oldStr.length);
  return p;
}
