import { Doc, type Op } from "./crdt";
import { diffCodePoints, mapIndex } from "./diff";

const room = new URLSearchParams(location.search).get("room") ?? "lobby";
const clientId = "c" + Math.random().toString(36).slice(2, 8);
const doc = new Doc(clientId);

const box = document.getElementById("editor") as HTMLTextAreaElement;
const status = document.getElementById("status") as HTMLElement;
const info = document.getElementById("info") as HTMLElement;
info.textContent = `room: ${room} | you are ${clientId}`;

const toCodePoints = (s: string): number[] => Array.from(s, (ch) => ch.codePointAt(0)!);

const ws = new WebSocket(`ws://${location.hostname}:8080/ws?room=${encodeURIComponent(room)}`);
const outbox: string[] = [];

ws.onopen = () => {
  status.textContent = "connected";
  status.className = "on";
  for (const msg of outbox.splice(0)) ws.send(msg);
};
ws.onclose = () => {
  status.textContent = "disconnected (reload to reconnect)";
  status.className = "off";
};

function send(op: Op): void {
  const msg = JSON.stringify(op);
  if (ws.readyState === WebSocket.OPEN) ws.send(msg);
  else outbox.push(msg);
}

// Remote ops that arrived before their dependency wait here.
let pending: Op[] = [];
function receive(op: Op): void {
  pending.push(op);
  for (let progress = true; progress; ) {
    progress = false;
    const still: Op[] = [];
    for (const p of pending) {
      if (doc.apply(p)) progress = true;
      else still.push(p);
    }
    pending = still;
  }
}

// Between events the text area always equals the document.
function render(): void {
  const next = doc.toString();
  const old = box.value;
  if (next === old) return;
  const start = box.selectionStart;
  const end = box.selectionEnd;
  box.value = next;
  box.setSelectionRange(mapIndex(old, next, start), mapIndex(old, next, end));
}

ws.onmessage = (ev) => {
  receive(JSON.parse(ev.data as string) as Op);
  render();
};

box.addEventListener("input", () => {
  const before = toCodePoints(doc.toString());
  const after = toCodePoints(box.value);
  const edit = diffCodePoints(before, after);
  for (let i = 0; i < edit.remove; i++) send(doc.localDelete(edit.start));
  edit.insert.forEach((cp, i) => send(doc.localInsert(edit.start + i, cp)));
});
