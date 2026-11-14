import argparse
import asyncio
import json
import random
import time
import urllib.request

import websockets


def pct(sorted_vals, p):
    if not sorted_vals:
        return float("nan")
    k = min(len(sorted_vals) - 1, int(round(p / 100 * (len(sorted_vals) - 1))))
    return sorted_vals[k]


async def user(idx, args, room, lat, counters, all_connected, go):
    cid = f"lt{idx}-{random.randrange(10**6)}"
    sent = {}  # ack key -> send time
    url = f"{args.url}/ws?room={room}"
    async with websockets.connect(url, max_size=None) as ws:
        counters["connected"] += 1
        if counters["connected"] == args.clients:
            all_connected.set()

        async def reader():
            try:
                async for msg in ws:
                    try:
                        m = json.loads(msg)
                    except ValueError:
                        continue
                    if isinstance(m, list) and len(m) == 2 and m[0] == "ack":
                        t0 = sent.pop(m[1], None)
                        if t0 is not None:
                            lat.append((time.perf_counter() - t0) * 1000)
                            counters["acked"] += 1
                    else:
                        counters["received"] += 1
            except websockets.ConnectionClosed:
                pass

        rtask = asyncio.create_task(reader())
        await go.wait()
        interval = 1.0 / args.rate
        await asyncio.sleep(random.random() * interval)  # spread the start
        prev = {"Client": "", "Seq": 0}
        for i in range(args.ops):
            seq = i + 1
            op = {
                "Type": 0,
                "ID": {"Client": cid, "Seq": seq},
                "After": prev,
                "Value": ord("a") + (i % 26),
                "Target": {"Client": "", "Seq": 0},
            }
            prev = op["ID"]
            key = f"0|{cid}:{seq}|:0"
            sent[key] = time.perf_counter()
            await ws.send(json.dumps(op))
            counters["sent"] += 1
            await asyncio.sleep(interval)
        # wait for outstanding acks
        deadline = time.perf_counter() + args.drain
        while sent and time.perf_counter() < deadline:
            await asyncio.sleep(0.05)
        counters["lost"] += len(sent)
        rtask.cancel()


def fetch_text(url, room):
    http = url.replace("ws://", "http://").replace("wss://", "https://")
    with urllib.request.urlopen(f"{http}/text?room={room}", timeout=10) as r:
        return r.read().decode("utf-8")


async def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", default="ws://localhost:8080")
    ap.add_argument("--clients", type=int, default=50)
    ap.add_argument("--ops", type=int, default=100, help="ops per client")
    ap.add_argument("--rate", type=float, default=10, help="ops/sec per client")
    ap.add_argument("--drain", type=float, default=10, help="seconds to wait for last acks")
    args = ap.parse_args()

    room = f"load-{int(time.time())}-{random.randrange(10**6)}"
    lat = []
    counters = {"connected": 0, "sent": 0, "acked": 0, "received": 0, "lost": 0}
    all_connected = asyncio.Event()
    go = asyncio.Event()

    tasks = [
        asyncio.create_task(user(i, args, room, lat, counters, all_connected, go))
        for i in range(args.clients)
    ]
    await asyncio.wait_for(all_connected.wait(), timeout=30)
    print(f"{args.clients} clients connected to room {room}")
    t0 = time.perf_counter()
    go.set()
    await asyncio.gather(*tasks)
    elapsed = time.perf_counter() - t0

    lat.sort()
    print()
    print(f"clients:          {args.clients}")
    print(f"ops sent:         {counters['sent']}")
    print(f"ops acked:        {counters['acked']}")
    print(f"ops lost:         {counters['lost']}")
    print(f"broadcasts recv:  {counters['received']}")
    print(f"duration:         {elapsed:.1f}s")
    print(f"throughput:       {counters['acked'] / elapsed:.0f} acked ops/s")
    print(f"ack latency ms:   p50={pct(lat,50):.1f}  p95={pct(lat,95):.1f}  p99={pct(lat,99):.1f}  max={lat[-1] if lat else float('nan'):.1f}")

    await asyncio.sleep(1)
    text = fetch_text(args.url, room)
    expected = args.clients * args.ops
    print(f"server text length: {len(text)} (expected {expected}) -> {'OK' if len(text) == expected else 'MISMATCH'}")


asyncio.run(main())
