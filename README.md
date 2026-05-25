# Redis Three-Modes Interactive Lab on Kubernetes

An interactive lab that runs the three canonical Redis topologies on a local
Kind cluster — **Standalone + Replication**, **Sentinel**, and **Cluster** —
and exposes a single-binary web UI to observe topology in real time, run
`redis-cli` commands against each lab, inject pod-kill faults, and watch
failover happen live.

Built as a learning playground for engineers about to write a Redis Kubernetes
operator: the deploy story is plain Bitnami helm charts (no operator yet), but
the backend uses the same primitives an operator would — `client-go` pod
watches, exec, helm wrappers.

```
+----------------------------------------------------+
| Navbar  [Standalone] [Sentinel] [Cluster]  [Reset] |
+----------------------------------------------------+
| TopologyView (SVG)                                 |
| - green master  - blue replica  - violet sentinel  |
| - amber sdown   - red odown/deleted                |
| - arrows: solid "repl" / dashed "monitor"          |
+--------------------+-------------------------------+
| RedisConsole       | FaultPanel                    |
| > SET foo bar      | [ kill master ]   (red)       |
| OK                 | [ kill replica-0 ]            |
| > GET foo          | --- Last kill: 4s ago         |
| "bar"              | --- Failover: completed in 6s |
+--------------------+-------------------------------+
| EventLog (SSE: pod_added/modified/deleted,         |
|           role_changed, sentinel_event)            |
+----------------------------------------------------+
```

---

## 1. What this is

A **demo-grade** local lab. Not production hardening: auth disabled, RBAC
permissive, no TLS, no NetworkPolicy. The point is to learn the *behavior* of
the three Redis topologies under failure, with a UI that makes the difference
visible.

Use it to:

- Watch standalone replication **break** when the master dies (no auto-promote).
- Watch Sentinel **switch master** in 5–10 s after kill.
- Watch Cluster **rebalance traffic** via MOVED/ASK, and verify keys with the
  same `{hash tag}` land in the same shard.

---

## 2. Architecture

```
                 ┌──────────────────────────────────────────────┐
                 │            Kind cluster (4 nodes)            │
                 │                                              │
   helm install  │  ┌──────────────────┐  ┌──────────────────┐  │
  ───────────►   │  │ ns redis-standalone │ ns redis-sentinel │  │
                 │  │ 1 master+2 replica│  │ 3 redis+sentinel │  │
                 │  └──────────────────┘  └──────────────────┘  │
                 │  ┌──────────────────┐                        │
                 │  │ ns redis-cluster │  (3 master + 3 replica)│
                 │  └──────────────────┘                        │
                 └────────────────┬─────────────────────────────┘
                                  │ kubectl port-forward
                                  ▼
                   ┌────────────────────────────────┐
                   │  bin/redis-lab (single binary) │
                   │   - Gin HTTP :8080             │
                   │   - client-go pod watch ×3     │
                   │   - sentinel PSUBSCRIBE        │
                   │   - redis & cluster clients    │
                   │   - SSE broadcaster            │
                   │   - embedded React UI          │
                   └────────────────┬───────────────┘
                                    ▼
                              browser :8080
```

| Mode        | Pods                                | Failover                   | Sharding | Notes                                                  |
|-------------|-------------------------------------|----------------------------|----------|--------------------------------------------------------|
| Standalone  | 1 master + 2 replica                | **none** (manual recovery) | none     | Replication ≠ HA. Kill master ⇒ writes break.          |
| Sentinel    | 3 pods, each runs redis + sentinel  | automatic, 5–10 s          | none     | Bitnami packs both in one pod; killing a pod kills both.|
| Cluster     | 3 master + 3 replica (6 pods)       | automatic, per shard       | 16384 slots, 3 shards | MOVED/ASK redirects on cross-shard keys.   |

---

## 3. Prerequisites

| Tool       | Min version | Notes                                       |
|------------|-------------|---------------------------------------------|
| Docker     | 24+         | Docker Desktop or daemon; ~4 GB RAM free.   |
| Kind       | 0.20+       | https://kind.sigs.k8s.io/                   |
| kubectl    | 1.28+       | Any 1.27+ works.                            |
| helm       | 3.13+       | v3 or v4.                                   |
| Go         | 1.22+       | For backend build.                          |
| Node.js    | 20+         | For UI build (`npm` 9+).                    |
| bash       | (any)       | Required for `scripts/*.sh`. Use Git Bash on Windows.|

---

## 4. Quick start

```bash
# 1. Bring up everything (Kind + 3 labs + UI + backend, runs in foreground)
make dev

# Then open: http://localhost:8080
```

Step by step if `make dev` is too magical:

```bash
make cluster-up       # create the Kind cluster
make deploy-all       # helm install standalone + sentinel + cluster
make backend-build    # npm install + vite build + go build → bin/redis-lab
make port-forward     # forwards 6379/26379/6381 to host (backgrounded)
./bin/redis-lab       # starts HTTP server on :8080 (UI embedded)
```

> **Windows users**: pass `EXE=.exe` to make targets that build the binary
> (e.g. `make backend-build EXE=.exe`) so the output is `bin/redis-lab.exe`.

Teardown:

```bash
make teardown   # uninstall helm releases + delete Kind cluster
make clean      # also remove bin/, node_modules/, dist/
```

---

## 5. What you'll see for each lab

### Standalone (`/standalone`)

Three pods: `redis-standalone-master-0` (green) and two `replicas-0/1` (blue),
connected by solid `repl` arrows pointing from master → replica. The
RedisConsole talks to the master service via port-forward; writes and reads
return immediately.

The teaching moment is **what does not happen** when you kill the master: no
new master is promoted. Writes fail until the StatefulSet recreates the pod
(~30 s). Concept: *replication is just data copy, not HA*.

### Sentinel (`/sentinel`)

Three pods (`redis-sentinel-node-0/1/2`), each running redis + sentinel in the
same container set. The current master is highlighted in green; the other two
are blue replicas. Below, three violet sentinel cards. Dashed `monitor` arrows
point from each sentinel up to the redis nodes.

When you kill the master, the event log lights up with sentinel pub/sub
messages — `+sdown` → `+odown` → `+switch-master mymaster <old> 6379 <new>
6379`. The topology updates within 5–10 s, and your next `SET` lands on the
new master automatically (data preserved, since replicas were in sync).

### Cluster (`/cluster`)

Three master shards across 16384 slots: `0–5460`, `5461–10922`, `10923–16383`.
Each shard has one replica. Below the topology a **SlotMap** ribbon shows the
slot ranges as colored segments — hover for the exact range.

Write `SET user:42 hello`. The console shows `slot=15880, executed on:
redis-cluster-2` plus a `MOVED 15880 → 10.244.2.4:6379` entry if you happened
to start from a node that doesn't own that slot. Try same-tag keys
(`user:{42}:profile` + `user:{42}:posts`) to verify they land in the same
shard; try `MGET a b` on unrelated keys for a `CROSSSLOT` error.

---

## 6. Demo scenarios

### Scenario A — Standalone: replication ≠ HA

1. Open `http://localhost:8080/standalone`.
2. Topology should show **1 green master + 2 blue replicas**.
3. Console:
   - `SET hello world` → `OK`.
   - `GET hello` → `world`.
4. FaultPanel → click **kill master** (red button), confirm.
5. Event log shows `[pod_deleted] redis-standalone-master-0`.
6. Topology turns the master red.
7. Console: `SET hello world2` → **error**: connection refused / lost. *This is
   the point.*
8. Wait ~30 s — StatefulSet recreates the pod, replication resyncs, master
   turns green again.
9. **Takeaway**: Standalone replication has no auto-promote. Adding more
   replicas does not give you HA.

### Scenario B — Sentinel: auto failover

1. Open `/sentinel`. Topology: 3 redis (1 master + 2 replicas) + 3 sentinels.
2. Console: `SET hello world` → `OK`.
3. Note the current master (e.g. `redis-sentinel-node-0`). FaultPanel → **kill
   master**.
4. Event log timeline (within 5–10 s):
   - `[pod_deleted] redis-sentinel-node-0`
   - `[sentinel_event] +sdown master mymaster ...`
   - `[sentinel_event] +odown master mymaster ...`
   - `[sentinel_event] +switch-master mymaster <old-host> 6379 <new-host> 6379`
   - `[role_changed]`
5. Topology updates: one replica is now green (new master).
6. Console: `GET hello` → `world` (data preserved). `SET hello world2` → `OK`
   (writes now go to the new master automatically via the FailoverClient).
7. ~30 s later, the killed pod is recreated; it joins as a replica.

### Scenario C — Cluster: sharding + MOVED redirect

1. Open `/cluster`. Topology: 6 pods (3 master + 3 replica). SlotMap below
   shows three colored segments.
2. Console: `SET user:42 hello` → `OK`, the result panel shows
   `slot=15880 · executed on 10.244.2.4:6379` and a `MOVED 15880 → ...` if a
   redirect happened.
3. Console: `SET user:{42}:profile alice` and `SET user:{42}:posts 100` — both
   land in the same slot because of the `{42}` hash tag. `MGET user:{42}:profile
   user:{42}:posts` → `["alice","100"]`.
4. Console: `MGET unrelated1 unrelated2` → `CROSSSLOT Keys in request don't
   hash to the same slot`.
5. FaultPanel → kill the master of shard 1 (e.g. `redis-cluster-0`).
6. Within 5–10 s the replica of that shard is promoted; SlotMap segment colors
   reflect the new layout when the topology refresh fires.
7. Console: `GET user:42` → still `hello` (data preserved).

**All three scenarios should pass cleanly. If one doesn't, see Troubleshooting
below.**

---

## 7. Troubleshooting

### Kind can't create the cluster
- Symptom: `make cluster-up` fails with `ERROR: failed to create cluster: …`.
- Causes & fixes:
  - **Docker daemon not running** — open Docker Desktop or `sudo systemctl
    start docker`.
  - **Not enough RAM** — Kind 4-node needs ~3 GB. Increase Docker Desktop
    memory to 6 GB+ in Settings → Resources.
  - **Old cluster name conflict** — `kind delete cluster --name redis-lab`
    then retry.

### Kind containers stopped after a Docker restart
- Symptom: `kubectl` fails with `dial tcp 127.0.0.1:PORT: connect: connection
  refused`.
- Cause: Kind nodes are normal Docker containers. Docker Desktop restart leaves
  them in `Exited`.
- Fix:
  ```bash
  docker start redis-lab-control-plane redis-lab-worker redis-lab-worker2 redis-lab-worker3
  # wait 1-2 min for API server + pods to recover
  ```

### Pods stuck Pending
- Symptom: `kubectl get pods -A | grep -v Running` shows pods Pending or
  ImagePullBackOff.
- Causes:
  - **Insufficient CPU/memory on worker nodes** — bump Docker memory.
  - **Image not found** — Bitnami changed distribution in Aug 2025; free tags
    moved to `bitnamilegacy/*`. The values files in `helm/` already use the
    `bitnamilegacy` registry. If you edit them, keep that registry override.

### Sentinel doesn't discover the master right after deploy
- Symptom: `/api/labs/sentinel/topology` returns `masterError: i/o timeout` for
  10–30 s after a fresh deploy.
- Cause: sentinels need to gossip with each other before they agree on the
  master. The backend retries every poll cycle, so the UI will catch up.
- Fix: wait 30 s. If still failing, `make reset-sentinel`.

### `kubectl port-forward` disconnects when you kill the target pod
- Symptom: After clicking "kill master" on the **sentinel** lab, sentinel
  commands suddenly time out.
- Cause: `kubectl port-forward svc/redis-sentinel` binds to **one** specific
  pod. Killing that pod severs the tunnel; Kubernetes' Service replicates the
  IP but the user-space PF process doesn't restart.
- Fix: restart the port-forward script:
  ```bash
  pkill -f 'port-forward.*redis-sentinel'
  kubectl port-forward -n redis-sentinel svc/redis-sentinel 26379:26379 &
  ```
  (Or just rerun `bash scripts/port-forward.sh`.)

### UI doesn't load / shows white screen
- Open dev console → check for errors.
- Confirm the backend is running: `curl http://localhost:8080/healthz` should
  return `{"ok":true}`.
- Confirm the UI was built and embedded: `ls backend/ui/dist` should contain
  `index.html` + `assets/`. If not, run `make ui-build` and rebuild backend.

### Redis modules error on standalone/sentinel pods
- Symptom: pod logs show `Module /opt/bitnami/redis/lib/redis/modules/redisearch.so
  failed to load`.
- Cause: recent Bitnami chart defaults `commonConfiguration` with `loadmodule`
  directives, but the `bitnamilegacy/redis:7.2.5` image doesn't ship those
  modules.
- Fix: the helm values files already override `commonConfiguration` to remove
  the loadmodule lines. If you edit the chart, keep that override.

### Cluster command returns NOAUTH
- Symptom: `redis-cli cluster info` returns `NOAUTH Authentication required`.
- Cause: `bitnami/redis-cluster` uses the key `usePassword`, NOT `auth.enabled`.
- Fix: `helm/values-cluster.yaml` sets `usePassword: false`. Don't accidentally
  flip it.

---

## 8. Next steps

This lab is intentionally a *behavior demo*, not an operator. Natural next
steps if you want to graduate it:

- **Wrap the helm calls in a kubebuilder operator** — turn `RedisStandalone`,
  `RedisSentinel`, `RedisCluster` into CRDs; replace `internal/k8s/helm.go`
  shell-out with declarative `Apply` of StatefulSets/Services.
- **Add Prometheus metrics** — Bitnami charts can enable `metrics:` block;
  point a Prometheus + Grafana at it for failover latency graphs.
- **NetworkPolicy partition test** — apply a `NetworkPolicy` that severs the
  network between sentinels and the master pod; observe the failover
  triggering on partition, not just on pod death.
- **Auth + TLS** — re-enable auth in the values files, plumb credentials
  through env vars, and verify SentinelClient / ClusterClient still work.
- **Backup/restore** — script a `BGSAVE` + PVC snapshot + restore loop, with a
  data-integrity check after restore.

---

## 9. License & warning

This project is provided as a learning sandbox under no specific license — copy
freely, attribute if useful.

**Do not run this against any cluster you care about.**

- The backend holds your **kubeconfig credentials in process**; with those
  perms it can `pods.delete` and `helm uninstall`. Don't expose `:8080`
  outside localhost.
- Auth is disabled on all three Redis labs. The data they hold is throwaway.
- The reset endpoint will wipe PVCs of the targeted lab; don't aim it at
  anything you want to keep.
