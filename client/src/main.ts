import { Doc, opKey, type Op } from "./crdt";
import { diffCodePoints, mapIndex } from "./diff";

const room = new URLSearchParams(location.search).get("room") ?? "lobby";
const clientId = "c" + Math.random().toString(36).slice(2, 8);
const doc = new Doc(clientId);
const wsBase = location.port === "5173" ? `ws://${location.hostname}:8080` : `${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}`;
const url = `${wsBase}/ws?room=${encodeURIComponent(room)}`;

const box = document.getElementById("editor") as HTMLTextAreaElement;
const status = document.getElementById("status") as HTMLElement;
const info = document.getElementById("info") as HTMLElement;

const btn = document.createElement("button");
btn.textContent = "Go offline";
info.after(btn);

const toCodePoints = (s: string): number[] => Array.from(s, (ch) => ch.codePointAt(0)!);

// Ops we sent that the server has not acknowledged yet, in the order we made them.
// After any reconnect we resend all of them; the server ignores duplicates.
const unacked = new Map<string, string>();

function updateInfo(): void {
  info.textContent = `room: ${room} | you are ${clientId} | unsynced ops: ${unacked.size} `;
}
updateInfo();

function setStatus(text: string, on: boolean): void {
  status.textContent = text;
  status.className = on ? "on" : "off";
}

let ws: WebSocket | null = null;
let retryDelay = 500;
let retryTimer: number | undefined;
let manualOffline = false;

function connect(): void {
  const sock = new WebSocket(url);
  ws = sock;
  sock.onopen = () => {
    retryDelay = 500;
    setStatus("connected", true);
    for (const msg of unacked.values()) sock.send(msg);
  };
  sock.onmessage = (ev) => {
    const data = JSON.parse(ev.data as string);
    if (Array.isArray(data)) {
      if (data[0] === "ack") {
        unacked.delete(data[1] as string);
        updateInfo();
      }
      return;
    }
    receive(data as Op);
    render();
  };
  sock.onclose = () => {
    if (ws !== sock) return;
    ws = null;
    if (manualOffline) {
      setStatus("offline (simulated)", false);
      return;
    }
    setStatus(`disconnected, retrying in ${(retryDelay / 1000).toFixed(1)}s`, false);
    retryTimer = window.setTimeout(connect, retryDelay);
    retryDelay = Math.min(retryDelay * 2, 5000);
  };
}

function send(op: Op): void {
  const msg = JSON.stringify(op);
  unacked.set(opKey(op), msg);
  updateInfo();
  if (ws && ws.readyState === WebSocket.OPEN) ws.send(msg);
}

btn.onclick = () => {
  manualOffline = !manualOffline;
  btn.textContent = manualOffline ? "Go online" : "Go offline";
  window.clearTimeout(retryTimer);
  if (manualOffline) {
    if (ws) ws.close();
    else setStatus("offline (simulated)", false);
  } else {
    retryDelay = 500;
    if (!ws) connect();
  }
};

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

box.addEventListener("input", () => {
  const before = toCodePoints(doc.toString());
  const after = toCodePoints(box.value);
  const edit = diffCodePoints(before, after);
  for (let i = 0; i < edit.remove; i++) send(doc.localDelete(edit.start));
  edit.insert.forEach((cp, i) => send(doc.localInsert(edit.start + i, cp)));
});

connect();
