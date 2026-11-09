# Benchmarks

Machine: AMD Ryzen 5 5600H, Go 1.27, Windows.

## Baseline (slice + linear scans)

| Benchmark | Time/op | Memory/op |
|---|---|---|
| Append 1k | 1.50 ms | 230 KB |
| Append 5k | 32.4 ms | 1.25 MB |
| Append 10k | 125.9 ms | 2.78 MB |
| Remote apply 5k | 19.0 ms | 1.25 MB |

Growth: 10x more characters costs 84x more time (quadratic).

## After optimization (linked list + ID map + cursor finger)

| Benchmark | Time/op | Memory/op | Allocs/op |
|---|---|---|---|
| Append 1k | 0.35 ms | 208 KB | 1,023 |
| Append 5k | 1.69 ms | 892 KB | 5,049 |
| Append 10k | 3.54 ms | 1.79 MB | 10,082 |
| Remote apply 5k | 1.69 ms | 892 KB | 5,049 |
| Random insert 5k | 24.4 ms | 898 KB | 5,050 |

Speedup at 10k appends: 125.9 ms to 3.54 ms (36x). Scaling is now roughly linear
(10x more characters costs about 10x more time, versus 84x before).

Remaining cost: random-position edits are O(distance from the last edit).
A balanced tree or skip list would fix this.
