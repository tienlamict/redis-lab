# Redis Three-Modes Interactive Lab — Tổng kết & Thiết kế

> Tài liệu tổng hợp: **kết quả đạt được**, **kiến trúc triển khai chi tiết**,
> và **cơ sở lý thuyết** của ba mô hình Redis được lab minh hoạ.
> Dùng làm cẩm nang khi onboard người mới hoặc làm base reference khi
> refactor dự án thành Operator.

---

## Mục lục

1. [Tổng quan & mục tiêu](#1-tổng-quan--mục-tiêu)
2. [Kết quả đạt được](#2-kết-quả-đạt-được)
3. [Kiến trúc triển khai](#3-kiến-trúc-triển-khai)
   - 3.1 [Tầng hạ tầng — Kind + Helm](#31-tầng-hạ-tầng--kind--helm)
   - 3.2 [Tầng backend — Go single binary](#32-tầng-backend--go-single-binary)
   - 3.3 [Tầng frontend — React SPA embedded](#33-tầng-frontend--react-spa-embedded)
   - 3.4 [Luồng dữ liệu end-to-end](#34-luồng-dữ-liệu-end-to-end)
4. [Cơ sở lý thuyết](#4-cơ-sở-lý-thuyết)
   - 4.1 [Redis Standalone + Replication](#41-redis-standalone--replication)
   - 4.2 [Redis Sentinel — High Availability](#42-redis-sentinel--high-availability)
   - 4.3 [Redis Cluster — Horizontal Scaling](#43-redis-cluster--horizontal-scaling)
   - 4.4 [Kubernetes primitives được dùng](#44-kubernetes-primitives-được-dùng)
   - 4.5 [Tại sao SSE thay vì WebSocket](#45-tại-sao-sse-thay-vì-websocket)
   - 4.6 [Mô hình client của go-redis v9](#46-mô-hình-client-của-go-redis-v9)
5. [Pitfalls đã giải quyết](#5-pitfalls-đã-giải-quyết)
6. [Giới hạn hiện tại & hướng phát triển](#6-giới-hạn-hiện-tại--hướng-phát-triển)

---

## 1. Tổng quan & mục tiêu

### Vấn đề muốn giải quyết

Khi học để xây Kubernetes Operator quản lý Redis, developer thường loay hoay
giữa 2 tài liệu rời nhau: docs Redis nói về replication/sentinel/cluster theo
góc nhìn của một process Redis đơn lẻ, còn docs Bitnami chart nói về cài đặt
helm chart theo góc nhìn YAML — **không có chỗ nào cho phép sờ tay vào và quan
sát hành vi thực tế** của ba mô hình khi gặp sự cố.

### Mục tiêu

Xây dựng một **interactive lab** chạy local (Kind) trong đó:

1. Cả ba mô hình Redis được deploy đồng thời, tách namespace.
2. Một web UI render topology realtime và cho phép gõ redis-cli command
   trực tiếp lên từng lab.
3. Cho phép inject fault (kill pod) bằng một click chuột.
4. Quan sát failover diễn ra qua **SSE event stream** kết hợp pod watch +
   sentinel pub/sub.
5. Reset từng lab độc lập, không cần down cluster.

### Đối tượng

- Developer background Go + intermediate K8s, đang chuẩn bị viết Operator.
- Cần demo trước team về sự khác biệt 3 mô hình mà không vấp deploy.
- Cần một base code có thể tiến hoá lên CRD + reconcile loop sau này.

### Phạm vi & nguyên tắc

- **Demo-grade**, KHÔNG production hardening: auth disabled, không TLS,
  RBAC permissive.
- **Idempotent**: mọi script và endpoint chạy lại nhiều lần không vỡ.
- **Single binary**: sau build, output là một `bin/redis-lab(.exe)` đã nhúng
  React UI — copy đi máy khác chạy được ngay.
- **No magic**: dùng Bitnami helm chart chính thức cho Redis, không gói lại;
  backend chỉ là wrapper REST + SSE quanh `client-go` và `go-redis`.

---

## 2. Kết quả đạt được

### 2.1 Bốn phase hoàn thành

| Phase | Phạm vi | Kết quả |
|-------|---------|---------|
| Phase 1 | Kind cluster (1 control-plane + 3 worker) + Helm install 3 chart Bitnami | 3 namespace `redis-standalone/sentinel/cluster` Ready; sentinel discovery xong; cluster `cluster_state:ok` |
| Phase 2 | Backend Go: pod watch, sentinel pub/sub, REST + SSE | 6 REST endpoint, 1 SSE stream/lab, `go vet ./...` clean, không panic |
| Phase 3 | React + TypeScript UI: topology SVG, console, fault panel, slot map, detail panel | TS strict pass, single-page app embedded vào Go binary |
| Phase 4 | Makefile, scripts, README 9 sections, acceptance scenarios A/B/C | `make backend-build` từ sạch OK, 3 scenarios end-to-end PASS qua UI binary |

### 2.2 Số liệu code

| Layer | Số file | LOC ước tính |
|-------|---------|-------------|
| Infra YAML + bash scripts | 4 + 5 | ~200 |
| Backend Go | 14 (1 binary entry, 5 packages) | ~1100 |
| Frontend TS/TSX | 16 (3 pages, 8 components, 3 hooks, types, api, entry) | ~1200 |
| Docs | README + DESIGN + comments inline | ~700 |
| **Tổng** | ~40 file | **~3200 LOC** |
| Binary output | 1 file `bin/redis-lab.exe` | 63 MB (gồm UI nhúng + go runtime + client-go) |

### 2.3 Acceptance scenarios — bằng chứng PASS

#### Scenario A — Standalone: replication ≠ HA

| Bước | Kết quả thực tế |
|------|----------------|
| `SET hello world` | `{"result":"OK"}` |
| `GET hello` | `{"result":"world"}` |
| Kill master `redis-standalone-master-0` | `{"message":"Pod redis-standalone-master-0 deleted","ok":true}` |
| `SET hello world2` ngay sau kill | `{"error":"dial tcp [::1]:6379: connectex: ... refused"}` |

→ Chứng minh standalone replication **không tự promote**; ứng dụng sẽ break writes
khi master chết.

#### Scenario B — Sentinel: auto failover

| Bước | Kết quả thực tế |
|------|----------------|
| Master trước kill | `redis-sentinel-node-0` |
| `SET hello world` | `{"result":"OK"}` |
| Kill `node-0` | Trong 3-5s, SSE phát các event: `+switch-master mymaster ... node-0 → node-1`, `role_changed`, `+sdown slave node-0`, `+sdown sentinel ...`, `pod_deleted`, `pod_added` |
| Master sau failover | `redis-sentinel-node-1` |
| `GET hello` | `"world"` (data còn) |
| `SET hello world2` | `OK` (gửi tới master mới) |

→ Chứng minh sentinel quorum-based election + failover hoàn tất < 10s,
data preserved nhờ async replication đã sync.

#### Scenario C — Cluster: sharding + MOVED + per-shard failover

| Bước | Kết quả thực tế |
|------|----------------|
| Topology | 3 shard: `cluster-0[0-5460] cluster-1[5461-10922] cluster-2[10923-16383]` |
| `SET user:42 hello` | `slot=15880`, MOVED → `cluster-2`, result OK |
| `SET user:{42}:profile alice` | `slot=8000` → cluster-1 |
| `SET user:{42}:posts 100` | `slot=8000` → cluster-1 (cùng hash tag) |
| `MGET user:{42}:profile user:{42}:posts` | `"alice\n100"` |
| `MGET unrelated1 unrelated2` | `CROSSSLOT Keys in request don't hash to the same slot` |
| Kill `cluster-1` | pod_deleted → pod_added events |
| `GET user:{42}:profile` sau failover | `"alice"` (data preserved) |
| `GET user:42` (slot 15880) | `"hello"` (shard khác không bị ảnh hưởng) |

→ Chứng minh sharding theo CRC16 mod 16384, hash tag `{X}` ép keys vào cùng
slot, CROSSSLOT bảo vệ tính nhất quán, per-shard failover độc lập với các
shard khác.

### 2.4 Quality requirements đạt

| Requirement spec | Bằng chứng |
|------|-----------|
| Idempotent | `helm upgrade --install` + `kubectl apply --dry-run=client \| kubectl apply` |
| Health check không sleep cứng | `helm --wait --timeout 5m` thay vì sleep |
| No panic Go | `grep panic\( backend/` → 0 matches |
| TypeScript strict, no `any` | `tsconfig: strict=true`, `grep any ui/src` → 0 matches |
| File header + godoc | Tất cả 14 file Go có comment block trước `package` |
| Single binary | `bin/redis-lab.exe` 63MB chạy standalone, UI embed |
| slog logging | `time=... level=INFO msg=...` format |
| Reset không cần down cluster | `helm uninstall → kubectl delete pvc → helm install` (1-2 phút) |

---

## 3. Kiến trúc triển khai

### 3.1 Tầng hạ tầng — Kind + Helm

#### Cấu hình Kind cluster

```yaml
# kind/cluster.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: redis-lab
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 30080   # dành sẵn cho NodePort UI (không dùng ở v1, dùng kubectl pf)
        hostPort: 30080
  - role: worker
  - role: worker
  - role: worker
```

3 worker để 6 pod cluster + 3 pod sentinel + 3 pod standalone spread đều,
mỗi node tải vừa phải. Control-plane tách riêng để API server không tranh
CPU với redis pods.

#### Bitnami chart matrix

| Lab | Chart | Tham số quan trọng | Pod xuất hiện |
|-----|-------|--------------------|---------------|
| standalone | `bitnami/redis` | `architecture: replication`, `sentinel.enabled: false`, `master.count: 1`, `replica.replicaCount: 2` | `redis-standalone-master-0`, `redis-standalone-replicas-0/1` |
| sentinel | `bitnami/redis` | `sentinel.enabled: true`, `replica.replicaCount: 3`, `sentinel.quorum: 2`, `downAfterMilliseconds: 5000`, `failoverTimeout: 10000` | `redis-sentinel-node-0/1/2` (mỗi pod = 2 container: redis + sentinel) |
| cluster | `bitnami/redis-cluster` | `cluster.nodes: 6`, `cluster.replicas: 1`, `cluster.init: true` | `redis-cluster-0..5` (3 master + 3 replica auto) |

**Image registry**: tất cả override `image.repository: bitnamilegacy/<name>`
vì Bitnami đã đổi distribution 8/2025 — free tag chuyển sang `bitnamilegacy/*`.

**Module override**: chart redis 25.x mặc định `loadmodule redisearch.so/rejson.so`
nhưng image 7.2.5 không ship modules → override `commonConfiguration` thành
`appendonly yes; save ""`.

#### Wrapper scripts

```
scripts/
├── setup.sh         # one-shot: kind create + deploy 3 lab
├── deploy-lab.sh    # idempotent: helm repo update + helm upgrade --install --wait
├── reset-lab.sh     # uninstall + delete PVCs + reinstall (1-2 phút)
├── port-forward.sh  # auto-detect svc names (pick_svc helper)
└── teardown.sh      # uninstall all + delete Kind cluster
```

Mọi script dùng `set -euo pipefail` và `helm upgrade --install` (idempotent) +
`kubectl create ns --dry-run=client -o yaml | kubectl apply -f -` để re-run an toàn.

---

### 3.2 Tầng backend — Go single binary

#### Cấu trúc package

```
backend/
├── cmd/server/main.go              # entry point, bootstrap mọi thứ
├── embed.go                        # //go:embed ui/dist
├── internal/
│   ├── events/broadcaster.go       # SSE pub/sub hub (per-lab channel)
│   ├── k8s/
│   │   ├── client.go               # load kubeconfig → Clientset + REST config
│   │   ├── pods.go                 # ListPods + WatchPods → SSE events
│   │   ├── exec.go                 # SPDY exec into pod (redis-cli)
│   │   └── helm.go                 # exec.Command wrapper cho helm CLI
│   ├── redis/
│   │   ├── standalone.go           # QueryRole, INFO replication parser
│   │   ├── sentinel.go             # FailoverClient + Sentinel admin + pub/sub
│   │   └── cluster.go              # CLUSTER NODES/INFO/KEYSLOT + exec-based exec
│   ├── fault/kill.go               # pod delete
│   └── api/
│       ├── router.go               # gin routes + Deps DI + SPA fallback
│       ├── topology.go             # 3 GET endpoints
│       ├── command.go              # POST execute redis command
│       ├── fault.go                # POST kill / reset
│       ├── sse.go                  # GET /sse/:lab streaming
│       └── static.go               # contentType helpers cho embed
└── go.mod
```

#### Dependency injection — Deps struct

```go
type Deps struct {
    K8s         *k8s.Client                 // shared clientset + rest config
    Broadcaster *events.Broadcaster         // SSE hub
    Standalone  *redislab.StandaloneClient  // → localhost:6379
    Sentinel    *redislab.SentinelClient    // → localhost:26379 + exec
    Cluster     *redislab.ClusterClient     // → localhost:6381 + exec
    NSStandalone, NSSentinel, NSCluster                string
    ReleaseStandalone, ReleaseSentinel, ReleaseCluster string
    ValuesStandalone, ValuesSentinel, ValuesCluster    string
}
```

Mọi handler nhận `*Deps`, không global state. Test friendly.

#### Pod watch → SSE

```go
// internal/k8s/pods.go
func (c *Client) WatchPods(ctx, ns, lab, bc) {
    for {
        w, _ := c.Set.CoreV1().Pods(ns).Watch(ctx, metav1.ListOptions{})
        for ev := range w.ResultChan() {
            pod := ev.Object.(*corev1.Pod)
            bc.Publish(events.Event{
                Lab:  lab,
                Type: mapWatchType(ev.Type), // pod_added/modified/deleted
                Payload: map[string]any{"pod": toPodInfo(pod)},
            })
        }
        // reconnect with 2s backoff on EOF
    }
}
```

3 goroutine watcher cho 3 namespace + 1 goroutine sentinel pub/sub →
broadcaster.

#### Broadcaster — non-blocking fanout

```go
// internal/events/broadcaster.go
type Broadcaster struct {
    subs map[string]map[chan Event]struct{}  // lab → set of channels
}

func (b *Broadcaster) Publish(e Event) {
    for ch := range b.subs[e.Lab] {
        select {
        case ch <- e:
        default: // drop if subscriber is slow; never block publisher
        }
    }
}
```

Buffer 16 per subscriber. Slow client drop events, không kẹt publisher.

#### Sentinel pub/sub — vì sao cần ngoài pod watch

Pod watch chỉ thấy lifecycle K8s. Sentinel quyết định ai là master qua
**internal election** (quorum + leader epoch); K8s không biết. Bitnami không
cập nhật pod label sau failover. Phải:

```go
// internal/redis/sentinel.go
pubsub := cli.PSubscribe(ctx,
    "+sdown", "-sdown",        // subjective down (1 sentinel thấy)
    "+odown", "-odown",        // objective down (quorum đồng ý)
    "+switch-master",          // master mới đã được chọn
    "+failover-state-*",       // chi tiết các bước failover
)
```

Đây là cách backend phát hiện `role_changed` thật, không phải qua poll.

#### Cluster client — vì sao không dùng ClusterClient của go-redis

`go-redis.NewClusterClient` tự discover topology qua `CLUSTER SLOTS`, lấy
được pod IPs nội bộ (10.244.x.x) — **không reachable từ host** qua
`kubectl port-forward`. Hai cách:

1. Port-forward riêng 6 endpoint pod → quá nhiều process.
2. Dùng plain `redis.Client` cho admin queries + `kubectl exec` cho commands.

Backend chọn (2). Command execution:

```go
// internal/redis/cluster.go
// Pass 1: redis-cli (KHÔNG -c) để bắt MOVED error rõ ràng
rawOut, _, _ := ExecFn(ctx, []string{"redis-cli", ...args})
parseRedirect(rawOut) → []Redirect

// Pass 2: redis-cli -c (cluster mode) để follow MOVED và lấy kết quả thật
stdout, _, _ := ExecFn(ctx, []string{"redis-cli", "-c", ...args})
parseRedisCliOutput(stdout) → result
```

Lý do 2 pass: `redis-cli -c` trong non-interactive mode **không in dòng
"-> Redirected"** — phải gọi không `-c` để nhìn thấy MOVED.

#### Sentinel command — exec workaround

Tương tự cluster: sentinel trả về master address dạng FQDN
`redis-sentinel-node-X.redis-sentinel-headless.redis-sentinel.svc.cluster.local`
không resolve từ host. Backend parse FQDN → pod name → `kubectl exec` vào
pod đó chạy `redis-cli`. Code:

```go
func (s *SentinelClient) CurrentMasterPod(ctx) (string, error) {
    addr, _ := s.MasterAddr(ctx)
    host := strings.Split(addr, ":")[0]    // strip port
    return strings.Split(host, ".")[0], nil // first label = pod name
}
```

#### HTTP layer — Gin + SSE

```go
r := gin.New()
r.Use(gin.Recovery(), gin.Logger())
r.GET("/api/labs/:lab/topology", topologyHandler)
r.POST("/api/labs/:lab/command", commandHandler)
r.POST("/api/labs/:lab/fault/kill", killHandler)
r.POST("/api/labs/:lab/reset", resetHandler)
r.GET("/sse/:lab", sseHandler)
r.NoRoute(spaFallback)  // serve embedded UI + SPA route fallback
```

SSE handler:

```go
c.Writer.Header().Set("Content-Type", "text/event-stream")
c.Writer.Header().Set("X-Accel-Buffering", "no")  // disable nginx buffering
ch, unsub := bc.Subscribe(lab); defer unsub()
flusher := c.Writer.(http.Flusher)
for {
    select {
    case <-c.Request.Context().Done(): return
    case ev := <-ch:
        fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", ev.Type, jsonBytes)
        flusher.Flush()
    case <-ticker.C:
        fmt.Fprint(c.Writer, ": ping\n\n")  // 15s keepalive
        flusher.Flush()
    }
}
```

---

### 3.3 Tầng frontend — React SPA embedded

#### Cây component

```
App
├── BrowserRouter
└── Navbar              [Standalone | Sentinel | Cluster]  [Reset {lab}]
    └── Routes
        ├── /standalone → StandalonePage
        ├── /sentinel   → SentinelPage
        ├── /cluster    → ClusterPage
        └── /           → redirect /standalone

each Page wraps:
LabLayout
├── TopologyView (SVG, lab-specific layout)
│   ├── onNodeClick → selected state
│   └── useFlash(signature) → flash-bg animation on role change
├── detailPanel (per page nội dung khác nhau)
├── extra (per page: amber/violet/cyan callout + SlotMap cho cluster)
├── RedisConsole
│   ├── useRedisCommand (history max 200)
│   ├── ArrowUp/Down navigation
│   └── CLEAR shortcut
├── FaultPanel
│   ├── per-pod kill button (rose cho master, amber cho replica)
│   ├── failover timer (start trên pod_deleted, stop trên role_changed)
│   └── last kill elapsed
└── EventLog
    └── color-coded by event type, max 200 lines, auto-scroll
```

#### State management

Không dùng Redux/Zustand — state cục bộ trong từng page là đủ:

```ts
// useTopology: poll + SSE-triggered refresh
const { data, refresh } = useTopology<L>(lab, refreshMs=4000)

// useSSE: EventSource với exponential backoff (1s → 30s max)
useSSE(lab, (event) => {
  setEvents(prev => [...prev.slice(-199), event])
  if (event.type !== 'hello') refresh()
})

// useRedisCommand: history list capped at 200
const { history, run, clear } = useRedisCommand(lab)
```

#### TopologyView — pure SVG, không thư viện ngoài

3 layout function:

```ts
StandaloneLayout(d): 1 master ở giữa trên + N replica dưới + arrow "repl"
SentinelLayout(d):  3 redis hàng trên (1 highlight master) + 3 sentinel hàng dưới + arrow "monitor" dashed
ClusterLayout(d):   N master hàng trên + N replica hàng dưới + SlotMap ribbon
```

Node = `<rect rx=10 width=140 height=60>` + 2 dòng text. Màu theo
`(status, role)`:

| status | role | màu |
|--------|------|-----|
| `ok` | `master` | green |
| `ok` | `slave/replica` | blue |
| `ok` | `sentinel` | violet |
| `pending`, `sdown` | * | amber |
| `odown`, `disconnected`, `error` | * | red |

Khi `selected` → border vàng, stroke 3px. Khi role thay đổi → CSS animation
`.flash-bg` 600ms vàng nhạt.

#### Vite + embedded build

```ts
// vite.config.ts — dev proxy
server: {
  proxy: {
    '/api':     'http://localhost:8080',
    '/sse':     { target: 'http://localhost:8080', ws: false },
    '/healthz': 'http://localhost:8080',
  },
}
```

Production:
```bash
npm run build        # → ui/dist/{index.html, assets/...}
cp -r ui/dist backend/ui/dist
go build -o bin/redis-lab ./cmd/server  # //go:embed ui/dist
```

Go `embed.FS` + `fs.Sub("ui/dist")` + `NoRoute` SPA fallback → 1 binary
phục vụ cả API và SPA, route `/sentinel`, `/cluster` đều trả về `index.html`.

---

### 3.4 Luồng dữ liệu end-to-end

#### Bootstrap

```
make dev
 ├─ kind create cluster --config kind/cluster.yaml
 ├─ helm upgrade --install redis-{standalone,sentinel,cluster} ...
 ├─ npm install && npm run build && cp ui/dist → backend/ui/dist
 ├─ go build → bin/redis-lab
 ├─ bash scripts/port-forward.sh &       # localhost:6379, 26379, 6381
 └─ ./bin/redis-lab                       # :8080
       ├─ load kubeconfig → Clientset
       ├─ 3× WatchPods goroutine (one per namespace)
       ├─ 1× WatchSentinelEvents goroutine (PSUBSCRIBE)
       └─ HTTP listen :8080
```

#### Request flow — kill master scenario

```
[browser]                       [backend :8080]                  [K8s API + Redis]

click "kill master"
POST /fault/kill {target:X} ─►  fault.Kill → pods.Delete(X) ────► API server
                                                                  delete pod
                            ◄── 200 {ok:true, message:...}
                                                                  StatefulSet
                                                                  controller
                                                                  detects, creates
                                                                  new pod

                                WatchPods sees Modified/Deleted ◄ watch event
                                bc.Publish(pod_deleted)
                                                                  pod entered
                                                                  Pending then
                                                                  Running

                                WatchPods sees Added/Modified  ◄ watch event
                                bc.Publish(pod_added)

[SSE stream alive]
event: pod_deleted    ◄────── flusher.Flush
event: pod_added      ◄──────

(if sentinel lab)
                                Sentinel PSUBSCRIBE channel    ◄ +switch-master
                                bc.Publish(sentinel_event)
                                bc.Publish(role_changed)

event: sentinel_event ◄──────
event: role_changed   ◄──────

useSSE callback → setEvents + refresh()
useTopology refetches → topology API → new master pod shown
TopologyView re-renders → useFlash detects role change → flash-bg 600ms
FaultPanel: failoverActive becomes false (role_changed received)
        → shows "Failover: completed in 4s"
```

---

## 4. Cơ sở lý thuyết

### 4.1 Redis Standalone + Replication

#### Mô hình

- Một master xử lý ghi (writes).
- N replica replicate từ master qua **async stream**:
  - Replica gửi `REPLCONF listening-port ... CAPA ...` → master.
  - Master gửi RDB snapshot + sau đó replication stream (mọi write command).
- Replication offset (`master_repl_offset`, `slave_repl_offset`) đo độ trễ.

```
   client writes
        │
        ▼
   ┌─master─┐
   │  AOF   │
   └───┬────┘
       │ async repl
       ▼
   ┌─replica─┐  ┌─replica─┐
   │  AOF    │  │  AOF    │
   └─────────┘  └─────────┘
```

#### Tại sao "replication ≠ HA"

Replication chỉ là **data copy**. Không có:

- **Leader election**: master chết → không ai promote replica.
- **Quorum check**: replica không biết master "thật sự chết" hay chỉ network blip.
- **Auto-reconfig**: ứng dụng vẫn cố connect tới IP master cũ → fail forever.

Để HA cần **bộ điều phối ngoài** quyết định: master có còn sống không, ai nên
được promote, làm thế nào để clients biết master mới. Đó chính là Sentinel.

#### Bitnami chart standalone

```yaml
architecture: replication
master.count: 1
replica.replicaCount: 2
sentinel.enabled: false
```

→ 3 StatefulSet pod (1 master + 2 replica). Service `<release>-master` chỉ
trỏ tới master (selector trên label `app.kubernetes.io/component=master`).
**Label này là static** trên StatefulSet → nếu master chết, service vẫn trỏ
vào pod chết tới khi K8s recreate. Không có promote tự động.

---

### 4.2 Redis Sentinel — High Availability

#### Mô hình

```
  ┌─sentinel─┐  ┌─sentinel─┐  ┌─sentinel─┐    (quorum = 2)
  └────┬─────┘  └────┬─────┘  └────┬─────┘
       │             │             │
       │  monitor    │  monitor    │  monitor
       ▼             ▼             ▼
  ┌─redis───┐  ┌─redis───┐  ┌─redis───┐
  │  master │←─│ replica │←─│ replica │
  └─────────┘  └─────────┘  └─────────┘
       ▲ async repl
```

#### Các giai đoạn failover

1. **SDOWN (Subjectively Down)** — Mỗi sentinel ping master mỗi 1s. Quá
   `down-after-milliseconds` (5000 trong lab này) không response → đánh dấu
   `+sdown`.
2. **ODOWN (Objectively Down)** — Sentinel hỏi các sentinel khác có thấy
   master down không. Nếu đủ **quorum** đồng ý → `+odown`.
3. **Leader election** — Sentinel với epoch cao hơn được vote làm leader cho
   failover này.
4. **Replica selection** — Leader chọn replica phù hợp nhất (replication
   offset cao, priority thấp, …).
5. **Promote** — Leader gửi `SLAVEOF NO ONE` cho replica được chọn.
6. **Reconfigure** — Các replica còn lại được redirect: `SLAVEOF <new-master>`.
7. **Notify** — Sentinel publish `+switch-master mymaster <old-host> <port> <new-host> <port>` trên pub/sub.
8. Clients subscribe sentinel sẽ nhận thông báo và reconnect tới master mới.

#### Quorum vs Number of sentinels

- **3 sentinels, quorum=2** là minimum để chịu được 1 sentinel chết (network
  partition).
- **2 sentinels, quorum=1** KHÔNG an toàn — split brain dễ xảy ra.
- **5 sentinels, quorum=3** dùng cho production khi multi-AZ.

#### Bitnami packaging

Bitnami chạy `redis` + `sentinel` trong **cùng pod** (multi-container). Pros:
- Co-located → giảm 1 service.
- Sentinel theo dõi local redis qua loopback.

Cons:
- Kill 1 pod = mất 1 redis + 1 sentinel cùng lúc. Phải có 3 pod để vẫn còn
  quorum.
- Restart cycle dài hơn (cả 2 container phải Ready).

#### Lý do `kubectl port-forward` chết khi kill master pod

`kubectl port-forward svc/<svc>` chọn **1 pod cụ thể** ở thời điểm start —
không round-robin lại như Service. Kill pod đó → tunnel sập. Đây là lý do
ta phải restart PF sau mỗi scenario kill-master.

Trong production: backend nên chạy **trong cluster** (không cần PF), dùng
internal DNS như `redis-sentinel.redis-sentinel.svc.cluster.local:26379`.
Backend sẽ tự discover sentinel mới qua DNS A record. Trong lab này ta
chấp nhận restart PF để giữ deploy đơn giản (binary chạy trên host).

---

### 4.3 Redis Cluster — Horizontal Scaling

#### Mô hình hash slot

- **16384 slots** (2^14, fit trong 2 bytes message).
- Mỗi key được map vào slot qua `CRC16(key) mod 16384`.
- Slots chia đều cho các master:
  - 3 master → slots `0-5460`, `5461-10922`, `10923-16383`.

#### Topology

```
client
   │
   ▼ TCP
┌──master-0─┐  ┌──master-1─┐  ┌──master-2─┐
│ slot 0-5460│ │ 5461-10922│  │10923-16383│
└─────┬──────┘ └─────┬─────┘  └─────┬─────┘
      │              │              │
      ▼              ▼              ▼
┌─replica─┐    ┌─replica─┐    ┌─replica─┐
└─────────┘    └─────────┘    └─────────┘

           ────── gossip bus (port 16379) ──────
```

#### Cluster bus protocol (port 16379)

- Mỗi node mở thêm port `data_port + 10000` cho gossip.
- Định kỳ gossip về 1/10 các node biết đến → eventually consistent topology.
- Phát hiện node fail qua heartbeat timeout.
- Khi master fail, replica của shard tự promote (no external coordinator).

#### MOVED / ASK redirect

```
client → master-0: SET user:42 hello
         master-0 tính CRC16("user:42") mod 16384 = 15880
         15880 ∉ master-0 → trả: (error) MOVED 15880 10.0.2.5:6379

client → 10.0.2.5: SET user:42 hello (master-2)
                   → OK
```

- **MOVED**: slot đã được migrate vĩnh viễn, client nên cache mapping mới.
- **ASK**: slot đang trong quá trình migrate, redirect tạm thời (không cache).
  Client gửi `ASKING` trước mỗi request follow ASK.

`redis-cli -c` (cluster mode) tự follow MOVED/ASK. Trong non-interactive mode
nó **không in dòng "-> Redirected"** — đây là lý do backend lab phải gọi
2 pass (không `-c` để bắt MOVED, rồi `-c` để lấy kết quả).

#### Hash tag — ép keys vào cùng slot

Multi-key operation (MGET, MSET, transactions, scripts) chỉ hoạt động khi
**tất cả key thuộc cùng 1 slot**. Hash tag là phần trong `{...}`:

- `user:42` → slot từ CRC16("user:42")
- `user:{42}:profile` → slot từ CRC16("42")  ← chỉ tính phần trong `{}`
- `user:{42}:posts` → slot từ CRC16("42")  ← cùng slot trên

→ `MGET user:{42}:profile user:{42}:posts` chạy được vì 2 key cùng slot 8000.

Không có hash tag thì `MGET unrelated1 unrelated2` → `CROSSSLOT Keys in
request don't hash to the same slot`. Đây là tính chất bảo toàn quan trọng
cho transactions.

#### Per-shard failover độc lập

Khi shard 1 master chết:
- Chỉ shard 1 mới failover (replica → master).
- Shard 0 và 2 không bị ảnh hưởng.
- Slot mapping được update qua gossip → các client/clients có cache cũ sẽ
  nhận MOVED → tự refresh cache.

→ Cluster cho horizontal scaling + per-shard HA. Trade-off: client phải
**cluster-aware** (hiểu MOVED/ASK), không phải client redis thông thường
nào cũng support.

---

### 4.4 Kubernetes primitives được dùng

| Primitive | Vai trò trong lab |
|-----------|---|
| **Namespace** | Cô lập 3 lab, dễ reset từng lab |
| **StatefulSet** | Stable pod identity `redis-cluster-0..5`, `redis-sentinel-node-0..2`, ordering quan trọng cho cluster init |
| **PersistentVolumeClaim** | Mỗi pod 1 PVC, reset-lab xoá để clear data |
| **Service ClusterIP** | DNS endpoint nội bộ; sentinel/cluster topology dựa vào hostname |
| **Service Headless** | DNS resolve thẳng pod IPs, cần cho sentinel/cluster gossip |
| **Watch API** (`pods.Watch()`) | Stream pod lifecycle events thay vì poll |
| **Exec API** (SPDY) | `kubectl exec` programmatic — chạy redis-cli trong pod khi network path không có |

#### Watch vs List+Poll

Backend dùng `Watch` thay vì poll vì:

| Watch | Poll |
|-------|------|
| Push-based, latency ≤ ms | Pull-based, latency = interval |
| 1 long-lived connection | N requests / interval |
| Auto-reconnect cần handle | Stateless, request lẻ |
| Native primitive K8s | Wasteful nếu interval ngắn |

#### SPDY exec — tại sao cần

Khi backend chạy ngoài cluster (host machine) và pod IPs không reachable,
exec là cách "trong cluster" duy nhất. SPDY protocol cho phép bidirectional
streaming stdin/stdout/stderr giống `kubectl exec`. Implementation trong
backend:

```go
exec.NewSPDYExecutor(config, "POST", req.URL()).StreamWithContext(ctx, opts)
```

Trade-off: mỗi call ~100-200ms overhead. Backend ưu tiên direct connection
(go-redis client over PF) cho hot path; exec chỉ dùng khi không có lựa chọn
(sentinel/cluster commands).

---

### 4.5 Tại sao SSE thay vì WebSocket

| Yêu cầu | SSE | WebSocket |
|---------|-----|-----------|
| Server → client only | ✅ native | overkill (full duplex) |
| Auto-reconnect | ✅ EventSource built-in | tự code |
| HTTP/1.1 + 2 compatible | ✅ | cần HTTP upgrade handshake |
| Cross-origin | CORS đơn giản | thêm header WS-specific |
| Library footprint | 0 (built-in fetch + EventSource) | thêm `ws` lib |
| Reverse proxy support | ✅ (nginx/cloudflare đều OK với event-stream) | nhiều khi cần config riêng |

Lab này chỉ cần push từ backend → browser (pod events, sentinel events). 0
nhu cầu client gửi message qua cùng connection (commands đi qua REST POST).
→ SSE là phù hợp nhất.

Implementation chú ý:
- Set `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no` (tắt nginx buffer).
- Mỗi event: `event: <type>\ndata: <json>\n\n`.
- Periodic `: ping\n\n` để giữ alive qua proxy timeout.
- Client: `EventSource` tự reconnect; backend lab thêm exponential backoff
  manual (1→30s) cho trường hợp DNS error hoặc backend restart.

---

### 4.6 Mô hình client của go-redis v9

#### 3 loại client

| Client | Dùng cho | Auto reconnect | Topology aware |
|--------|---------|----------------|----------------|
| `redis.NewClient` | Standalone | có (pool) | không |
| `redis.NewFailoverClient` | Sentinel | có; query sentinel mỗi lần connect | có (qua sentinel) |
| `redis.NewClusterClient` | Cluster | có; cache CLUSTER SLOTS, refresh khi MOVED | có (gossip + MOVED) |

#### FailoverClient hoạt động ra sao

1. Khi cần connection mới, gọi `sentinel get-master-addr-by-name mymaster`.
2. Nhận `(host, port)` của master hiện tại.
3. Dial tới host:port.
4. Cache kết quả trong pool, dùng cho subsequent requests.
5. Khi nhận lỗi (master changed), invalidate cache → query lại sentinel.

**Pitfall trong lab**: sentinel trả host dạng FQDN `pod.headless.ns.svc...`
không resolve từ host machine. FailoverClient dial fail. Backend lab giải
quyết bằng exec workaround (parse pod name từ FQDN → kubectl exec).

#### ClusterClient hoạt động ra sao

1. Khi start, query `CLUSTER SLOTS` từ seed address.
2. Cache mapping: slot → master IP, replicas.
3. Mỗi command → compute slot từ key → gửi tới master tương ứng.
4. Nếu nhận MOVED → retry trên target + invalidate cache.
5. Nếu nhận ASK → gửi ASKING + command tới target (không cache).

**Pitfall**: discover IPs nội bộ pod không reachable từ host → backend lab
dùng plain `redis.Client` + exec.

#### PubSub

```go
pubsub := cli.PSubscribe(ctx, "+switch-master", ...)
defer pubsub.Close()
for msg := range pubsub.Channel() {
    // msg.Channel, msg.Payload
}
```

Lab dùng PubSub cho sentinel events (cách duy nhất để biết failover real-time
ngoài polling `SENTINEL MASTER`).

---

## 5. Pitfalls đã giải quyết

| # | Pitfall | Giải pháp |
|---|---------|----------|
| 1 | Bitnami chuyển distribution 8/2025, free tags `bitnami/redis:7.2.5-debian-12-r0` không còn | Override `image.repository: bitnamilegacy/<name>` + `global.security.allowInsecureImages: true` |
| 2 | Chart 25.x default `commonConfiguration` load modules redisearch/rejson không có trong image 7.2.5 | Override `commonConfiguration` chỉ giữ `appendonly yes; save ""` |
| 3 | Chart `bitnami/redis-cluster` dùng key `usePassword` chứ KHÔNG `auth.enabled` | Đổi sang `usePassword: false` |
| 4 | `go-redis.ClusterClient` discover pod IPs (10.244.x.x) không reachable từ host qua PF | Dùng plain `redis.Client` cho admin queries + `kubectl exec` cho command execution |
| 5 | Sentinel FailoverClient trả master FQDN không resolve từ host | Parse FQDN → pod name → exec redis-cli vào pod |
| 6 | `redis-cli -c` non-interactive không in "-> Redirected" lines | Chạy 2 pass: không `-c` để bắt MOVED, rồi `-c` để lấy result thật |
| 7 | `kubectl port-forward` bám cứng 1 pod; kill pod → PF chết, sentinel pubsub mất connection | Document trong README troubleshooting; backend có exponential backoff reconnect; user restart PF khi cần |
| 8 | Kind container `Exited` sau Docker Desktop restart | `docker start redis-lab-control-plane redis-lab-worker{,2,3}` + wait pods ready (2-3 phút) |
| 9 | Sentinel pod cần ~30s sau deploy để discover master qua gossip | Backend retry mỗi poll cycle (4s); UI tự bắt kịp |
| 10 | Vite dev proxy ban đầu nghi ngờ buffer SSE → test thấy hoạt động OK | Confirm `Content-Type: text/event-stream` passthrough; chỉ cần `X-Accel-Buffering: no` ở backend |
| 11 | FaultPanel hiển thị nhầm role: cluster có 3 master, prop `masterPod: string` chỉ highlight 1 | Đổi `masterPods?: string[]`, cluster page truyền `shards.map(s => s.masterPod)` |
| 12 | Schema mismatch giữa backend response và spec (Phase 2 ban đầu trả `addr`, spec yêu cầu `ip`; trả `name/flags` thay vì `pod/status/monitoring`) | Refactor `topology.go` thêm `podNameFromFQDN`, `podForIP`, `flagStatus` helpers; chuẩn hoá tên field |
| 13 | `make port-forward` không có `&` → chain `make port-forward && ./bin/redis-lab` hang | Thêm `&` vào recipe; tạo `port-forward-fg` cho foreground |

---

## 6. Giới hạn hiện tại & hướng phát triển

### Giới hạn (cố ý, vì là demo-grade)

- **Auth disabled** trên cả 3 lab → bất cứ ai chạm vào port-forward cũng đọc/ghi được.
- **Không TLS** trong cluster.
- **Backend chạy ngoài cluster**, dùng kubeconfig của user → quyền tương đương user
  trên Kind cluster (cluster-admin). Đừng expose `:8080` ra ngoài localhost.
- **Port-forward fragile**: kill master → PF chết → cần restart. Trong production
  backend chạy trong cluster sẽ né được vấn đề này.
- **Reset xoá PVC** → toàn bộ data lab đó mất. Có cảnh báo confirm trong UI.

### Kiến trúc cho phép tiến hoá

Backend đã dùng các primitive K8s native (watch, exec, helm via shell-out).
Nếu chuyển lên operator chỉ cần:

1. Định nghĩa CRD: `RedisStandalone`, `RedisSentinel`, `RedisCluster`.
2. Reconcile loop dùng `controller-runtime` (kubebuilder).
3. Replace `internal/k8s/helm.go` shell-out bằng `Apply()` các StatefulSet/Service
   tự sinh.
4. Move backend vào in-cluster deployment → bỏ port-forward.
5. Giữ nguyên UI + API contract; chỉ thay implementation backend.

### Hướng phát triển gợi ý (thứ tự ưu tiên)

| # | Hạng mục | Giá trị | Effort |
|---|---------|---------|--------|
| 1 | Move backend in-cluster (Deployment + ServiceAccount + RBAC tối thiểu) | Bỏ pitfall PF, gần production hơn | M |
| 2 | Thêm `metrics.enabled: true` cho 3 chart + Prometheus + Grafana dashboard | Quan sát failover latency, repl lag | M |
| 3 | Refactor thành Operator (kubebuilder) với 3 CRD | Học được pattern thật | L |
| 4 | NetworkPolicy partition test (cô lập sentinel khỏi master để trigger failover qua partition chứ không phải pod death) | Edge case quan trọng cho sentinel | S |
| 5 | Bật auth + TLS, plumb credentials qua env vars | Production-ish | M |
| 6 | Backup/restore loop: BGSAVE + PVC snapshot + restore + integrity check | Operator chuẩn cần có | L |
| 7 | E2E test script tự động chạy 3 scenarios A/B/C, assert events qua SSE | CI pipeline ready | M |
| 8 | Thêm trang `/compare` so sánh 3 mode side-by-side với cùng workload | Demo trực quan hơn | S |

---

## Kết luận

Lab này hoàn thành 4 phase đúng spec, 3 acceptance scenarios PASS qua UI
single-binary, code đạt yêu cầu quality (no panic, no `any`, idempotent
scripts, slog logging). Quan trọng hơn, **kiến trúc đã được chuẩn bị sẵn cho
bước tiến hoá** — chuyển backend vào in-cluster + refactor thành Operator
chỉ cần thay implementation, không cần rewrite UI hay API contract.

Tài liệu này (`DESIGN.md`) + `README.md` là cẩm nang đủ để một developer
mới onboard mà không cần hỏi lại.
