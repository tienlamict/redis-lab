# Project: Redis Three-Modes Interactive Lab on Kubernetes

Hãy xây dựng một **interactive lab** chạy trên Kind (local Kubernetes) để minh hoạ và tương tác với 3 mô hình Redis: **Standalone with Replication**, **Sentinel**, và **Cluster**. Mục tiêu: demo được trước team, chạy đi chạy lại nhiều lần, và làm base code để sau này có thể phát triển thành Redis Operator riêng.

## Đối tượng và mục đích

- **Người dùng**: developer đang học để xây Kubernetes Operator quản lý Redis (background Go, hiểu K8s ở mức intermediate).
- **Use case chính**: chạy lab → mở UI trong browser → quan sát topology real-time → gõ Redis command → click "kill master" → xem failover diễn ra → reset lab → chạy lại. Lặp lại để hiểu sâu hành vi của từng mô hình.
- **Demo-grade**: code sạch, idempotent, dễ chạy đi chạy lại, error message thân thiện. KHÔNG cần production hardening (auth, TLS, RBAC chi tiết).

## Tech stack bắt buộc

- **Local K8s**: Kind v0.20+, cluster config 1 control-plane + 3 worker
- **Redis deployment**: **Bitnami Helm charts** (KHÔNG tự viết YAML, KHÔNG dùng third-party operator)
  - Mode 1 & 2: chart `bitnami/redis` (cùng chart, khác config)
  - Mode 3: chart `bitnami/redis-cluster` (chart riêng biệt)
- **Redis version**: 7.2.x (set qua chart values)
- **Backend**: Go 1.22+, Gin (HTTP framework), `client-go` (K8s API), `github.com/redis/go-redis/v9`
- **Frontend**: React 18 + TypeScript + Vite + Tailwind CSS
- **Real-time**: Server-Sent Events (SSE), KHÔNG WebSocket
- **Single binary**: dùng `embed.FS` để nhúng React build vào Go binary

## Cấu trúc thư mục chính xác

```
redis-lab-k8s/
├── kind/
│   └── cluster.yaml
├── helm/
│   ├── values-standalone.yaml      # bitnami/redis với replication
│   ├── values-sentinel.yaml        # bitnami/redis với sentinel.enabled=true
│   └── values-cluster.yaml         # bitnami/redis-cluster
├── backend/
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── api/
│   │   │   ├── router.go
│   │   │   ├── topology.go         # GET topology cho 3 lab
│   │   │   ├── command.go          # POST execute redis command
│   │   │   ├── fault.go            # POST kill pod / reset lab
│   │   │   └── sse.go              # SSE event stream
│   │   ├── k8s/
│   │   │   ├── client.go           # tạo clientset từ kubeconfig
│   │   │   ├── pods.go             # list, watch pods theo namespace
│   │   │   ├── exec.go             # exec command vào pod (cho redis-cli ROLE)
│   │   │   └── helm.go             # wrapper helm install/uninstall via exec.Command
│   │   ├── redis/
│   │   │   ├── standalone.go       # NewClient + role detection
│   │   │   ├── sentinel.go         # NewFailoverClient + sentinel pub/sub
│   │   │   └── cluster.go          # NewClusterClient + slot/shard inspection
│   │   ├── fault/
│   │   │   └── kill.go             # delete pod = "kill master"
│   │   └── events/
│   │       └── broadcaster.go      # SSE hub: 1 publisher → N subscribers
│   ├── ui/dist/                    # build output của React (gitignore)
│   ├── embed.go                    # //go:embed ui/dist
│   ├── go.mod
│   └── go.sum
├── ui/
│   ├── src/
│   │   ├── pages/
│   │   │   ├── Standalone.tsx
│   │   │   ├── Sentinel.tsx
│   │   │   └── Cluster.tsx
│   │   ├── components/
│   │   │   ├── TopologyView.tsx    # SVG topology
│   │   │   ├── NodeCard.tsx
│   │   │   ├── RedisConsole.tsx    # REPL input
│   │   │   ├── EventLog.tsx        # streaming log
│   │   │   ├── FaultPanel.tsx      # kill buttons + reset
│   │   │   ├── SlotMap.tsx         # chỉ cluster: 16384 slots ribbon
│   │   │   └── Navbar.tsx
│   │   ├── hooks/
│   │   │   ├── useSSE.ts
│   │   │   ├── useTopology.ts
│   │   │   └── useRedisCommand.ts
│   │   ├── api.ts                  # fetch wrappers
│   │   ├── types.ts
│   │   ├── App.tsx
│   │   └── main.tsx
│   ├── index.html
│   ├── package.json
│   ├── vite.config.ts
│   ├── tailwind.config.js
│   ├── postcss.config.js
│   └── tsconfig.json
├── scripts/
│   ├── setup.sh                    # one-shot: cluster + deploy + start
│   ├── teardown.sh
│   ├── deploy-lab.sh               # arg: standalone|sentinel|cluster
│   ├── reset-lab.sh                # uninstall + reinstall chart cho 1 lab
│   └── port-forward.sh             # mở port-forward cho 3 lab + UI
├── Makefile
├── .gitignore
└── README.md
```

## Phase 1: Kind cluster + Helm deployment

### `kind/cluster.yaml`

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: redis-lab
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 30080   # UI NodePort
        hostPort: 30080
        protocol: TCP
  - role: worker
  - role: worker
  - role: worker
```

### Helm values

**`helm/values-standalone.yaml`** (chart `bitnami/redis`, architecture replication, KHÔNG sentinel):
```yaml
architecture: replication
auth:
  enabled: false                # disable cho dễ demo
master:
  count: 1
  resources:
    requests: { cpu: 100m, memory: 128Mi }
replica:
  replicaCount: 2
  resources:
    requests: { cpu: 100m, memory: 128Mi }
sentinel:
  enabled: false
image:
  tag: 7.2.5-debian-12-r0
```

**`helm/values-sentinel.yaml`** (cùng chart `bitnami/redis`, BẬT sentinel — chú ý: khi sentinel.enabled=true, Bitnami chạy redis + sentinel TRONG CÙNG MỘT POD):
```yaml
architecture: replication
auth:
  enabled: false
replica:
  replicaCount: 3               # tổng 3 pod, mỗi pod có redis + sentinel sidecar
sentinel:
  enabled: true
  quorum: 2
  downAfterMilliseconds: 5000   # cho demo nhanh
  failoverTimeout: 10000
image:
  tag: 7.2.5-debian-12-r0
```

**`helm/values-cluster.yaml`** (chart **khác hoàn toàn**: `bitnami/redis-cluster`):
```yaml
auth:
  enabled: false
cluster:
  nodes: 6                      # 3 master + 3 replica auto
  replicas: 1
  init: true                    # tự chạy cluster init
image:
  tag: 7.2.5-debian-12-r0
```

### Mỗi lab vào 1 namespace riêng

- `redis-standalone`, `redis-sentinel`, `redis-cluster`.
- Tên release Helm trùng tên namespace cho dễ nhớ.

### `scripts/deploy-lab.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
LAB="${1:?usage: deploy-lab.sh standalone|sentinel|cluster}"
NS="redis-${LAB}"

helm repo add bitnami https://charts.bitnami.com/bitnami 2>/dev/null || true
helm repo update bitnami

kubectl create namespace "${NS}" --dry-run=client -o yaml | kubectl apply -f -

if [[ "${LAB}" == "cluster" ]]; then
  CHART="bitnami/redis-cluster"
else
  CHART="bitnami/redis"
fi

helm upgrade --install "${NS}" "${CHART}" \
  --namespace "${NS}" \
  --values "helm/values-${LAB}.yaml" \
  --wait --timeout 5m

kubectl get pods -n "${NS}" -o wide
```

### Validation cuối Phase 1

```bash
make cluster-up
./scripts/deploy-lab.sh standalone
./scripts/deploy-lab.sh sentinel
./scripts/deploy-lab.sh cluster
kubectl get pods -A | grep redis-
```

Tất cả pod Ready. Test sentinel failover thủ công bằng:
```bash
kubectl exec -n redis-sentinel redis-sentinel-node-0 -c sentinel -- redis-cli -p 26379 sentinel master mymaster
```

Nếu cả 3 lab Ready và sentinel command trả về thông tin master → Phase 1 xong. **Không chuyển sang Phase 2 nếu chưa đạt mốc này.**

## Phase 2: Backend Go

### `backend/cmd/server/main.go`

- Đọc kubeconfig từ `~/.kube/config` (hoặc env `KUBECONFIG`).
- Khởi tạo: K8s clientset, event broadcaster, pod watcher cho 3 namespace.
- Pod watcher mỗi namespace → push event vào SSE broadcaster theo channel `lab:<name>`.
- HTTP server `:8080`:
  - `/api/*` → REST
  - `/sse/:lab` → SSE
  - `/*filepath` → static files từ embedded UI (fallback to `index.html` cho SPA routing)

### REST API spec (chính xác)

```
GET  /api/labs/standalone/topology
  → {
      master: { pod, ip, role, status, replOffset },
      replicas: [{ pod, ip, role, status, replOffset }],
    }

GET  /api/labs/sentinel/topology
  → {
      master: { pod, ip, role, status },
      replicas: [...],
      sentinels: [{ pod, status, monitoring }],
      currentMaster: "redis-sentinel-node-0",   # từ sentinel master mymaster
    }

GET  /api/labs/cluster/topology
  → {
      shards: [{
        masterId, masterPod, slotRanges: [[0, 5460]],
        replicas: [{ id, pod }],
      }],
      nodes: [...],   # raw cluster nodes list để debug
    }

POST /api/labs/:lab/command
  body: { command: "SET foo bar" }
  → {
      result: "OK",
      executedOn: "redis-standalone-master-0",
      slot: 7000,        # chỉ có khi lab=cluster
      redirects: [],     # MOVED/ASK redirect chain
    }

POST /api/labs/:lab/fault/kill
  body: { target: "pod-name" }
  → { ok: true, message: "Pod redis-standalone-master-0 deleted" }

POST /api/labs/:lab/reset
  → { ok: true, message: "Lab standalone reset" }
  # = helm uninstall + helm install lại

GET  /sse/:lab → text/event-stream
  events:
    pod_added | pod_modified | pod_deleted
    role_changed
    sentinel_event   (subset: +sdown, -sdown, +odown, +switch-master, +failover-state-*)
    cluster_event    (subset: cluster bus messages nếu khả thi, hoặc CLUSTER NODES diff)
```

### `internal/redis/standalone.go` — role detection

Bitnami KHÔNG gắn role label trên pod (pod chỉ có label kiểu `app.kubernetes.io/component=master` hoặc `replica` theo chart, nhưng cái này là static, không phản ánh sau failover). Phải dùng `redis-cli ROLE` qua exec hoặc connect:

```go
// Connect tới Service ClusterIP, gọi ROLE để biết master/replica thật sự.
// Hoặc exec vào pod: kubectl exec <pod> -- redis-cli ROLE
// Khuyến nghị: connect qua Service vì nhanh hơn exec.
```

### `internal/redis/sentinel.go` — quan trọng

Phát hiện thay đổi master qua **2 nguồn song song**:

1. **K8s pod watch** (đã có ở Phase 2 setup).
2. **Sentinel pub/sub channels** — subscribe tới các channel sau và push thành SSE event:
   - `+sdown` / `-sdown`
   - `+odown` / `-odown`
   - `+switch-master`
   - `+failover-state-*`

Cách subscribe:
```go
// Tới sentinel pod qua Service, dùng PSUBSCRIBE "*" hoặc subscribe các channel cụ thể.
// Mỗi message → parse → tạo SSE event { type: "sentinel_event", channel: "+switch-master", payload: "..." }
```

**Lưu ý**: Bitnami chạy redis + sentinel cùng pod, sentinel port mặc định 26379. Service mặc định của Bitnami sentinel chart: `redis-sentinel` (port 26379). Backend connect vào đây.

### `internal/redis/cluster.go`

Helper bắt buộc:
```go
GetSlotForKey(ctx, key) (int, error)              // CLUSTER KEYSLOT
GetShardInfo(ctx) ([]ShardInfo, error)            // parse CLUSTER SHARDS
GetClusterNodes(ctx) ([]NodeInfo, error)          // parse CLUSTER NODES
ExecuteWithRedirectLog(ctx, cmd) (Result, []Redirect, error)
```

`ExecuteWithRedirectLog`: chạy command, nếu nhận `MOVED slot ip:port` → log redirect, retry tới node đúng, trả về cả result + chain redirect cho UI hiển thị.

### `internal/k8s/helm.go`

Wrapper đơn giản dùng `exec.Command("helm", ...)`. Không cần Helm SDK Go. Hàm:
```go
Install(release, chart, namespace, valuesFile string) error
Uninstall(release, namespace string) error
```

### `internal/fault/kill.go`

`Kill(namespace, podName)` = `clientset.CoreV1().Pods(ns).Delete(...)`. Pod watcher sẽ tự push event ra UI.

## Phase 3: UI React

### Layout chung mỗi page (3 page: Standalone, Sentinel, Cluster)

```
┌─────────────────────────────────────────────────────┐
│ Navbar: [Standalone] [Sentinel] [Cluster]  [Reset]  │
├─────────────────────────────────────────────────────┤
│                                                     │
│  TopologyView (~50% chiều cao)                      │
│  - SVG, các node là rounded rect                    │
│  - Màu theo trạng thái: green/amber/red             │
│  - Arrow replication, monitor (sentinel)            │
│  - Click node → detail panel bên phải               │
│                                                     │
├──────────────────────┬──────────────────────────────┤
│                      │                              │
│  RedisConsole        │  FaultPanel                  │
│  > SET foo bar       │  [ Kill master ]  (red)      │
│  OK                  │  [ Kill replica-0 ]          │
│  > GET foo           │  [ Kill replica-1 ]          │
│  "bar"               │  ──────                      │
│                      │  Last kill: 12s ago          │
│                      │  Failover: completed         │
│                      │                              │
├──────────────────────┴──────────────────────────────┤
│ EventLog (scrolling, color-coded, max 200 lines)    │
│  10:23:45 [pod] master deleted (redis-0)            │
│  10:23:48 [sentinel] +sdown master mymaster ...     │
│  10:23:50 [sentinel] +switch-master ... redis-1     │
└─────────────────────────────────────────────────────┘
```

### TopologyView component

- Pure SVG, không cần thư viện ngoài (KHÔNG dùng React Flow).
- Node: rounded rect 140×60, label = pod name + role.
- Màu: 
  - green: master healthy
  - blue: replica healthy
  - amber: SDOWN / pending
  - red: deleted / failed
- Arrow: 
  - solid với label "repl" cho replication (master → replica)
  - dashed với label "monitor" cho sentinel → redis
- Animation:
  - Pod xóa: fade out 400ms.
  - Pod mới: fade in 400ms.
  - Role thay đổi: flash background 600ms.
- Layout:
  - Standalone: master ở giữa trên, 2 replica dưới.
  - Sentinel: hàng trên redis (3 pod), hàng dưới sentinel (3 pod).
  - Cluster: 2 hàng — hàng master, hàng replica, dùng `<SlotMap>` riêng phía dưới.

### SlotMap (chỉ Cluster page)

Thanh ngang dài, chia 16384 slots thành các segment theo shard. Mỗi shard màu khác. Hover hiện range. Khi failover → animation segment đổi sang màu của master mới.

### RedisConsole

- Input giống terminal, prompt `>`.
- History navigation bằng ArrowUp/Down.
- Send command: POST `/api/labs/:lab/command`.
- Hiển thị output dạng monospace.
- **Cluster mode**: dưới mỗi result hiện `slot=N, executed on: pod-name`. Nếu có redirect: hiện `MOVED 7000 → redis-cluster-2` ở dòng dimmed.
- Built-in shortcuts: gõ `CLEAR` → clear console (client-side, không gửi server).

### FaultPanel

- Tự động fetch danh sách pod từ topology API.
- Group theo role: master button có viền/nền đỏ đậm, replica button nền cam nhạt.
- Click → confirm modal → POST `/api/labs/:lab/fault/kill`.
- Sau khi kill: hiện timer "elapsed: 3s, 4s, 5s..." cho đến khi nhận được event `role_changed` hoặc `+switch-master`.
- Nút "Reset lab" (đặt ở navbar luôn): confirm → POST `/api/labs/:lab/reset` → spinner 1-2 phút → reload page.

### useSSE hook

Auto-reconnect với exponential backoff (1s, 2s, 4s, max 30s). Parse event JSON, dispatch vào reducer hoặc Zustand store.

### Tailwind config

Dùng default Tailwind palette. Không cần custom theme. Dark mode optional (nice to have nhưng không bắt buộc).

## Phase 4: Scripts, Makefile, README

### `Makefile`

```make
.PHONY: cluster-up cluster-down deploy-standalone deploy-sentinel deploy-cluster deploy-all
.PHONY: ui-build backend-build dev clean teardown port-forward

cluster-up:
	kind create cluster --config kind/cluster.yaml

cluster-down:
	kind delete cluster --name redis-lab

deploy-standalone:
	./scripts/deploy-lab.sh standalone

deploy-sentinel:
	./scripts/deploy-lab.sh sentinel

deploy-cluster:
	./scripts/deploy-lab.sh cluster

deploy-all: deploy-standalone deploy-sentinel deploy-cluster

ui-build:
	cd ui && npm install && npm run build
	rm -rf backend/ui/dist
	mkdir -p backend/ui
	cp -r ui/dist backend/ui/dist

backend-build: ui-build
	cd backend && go build -o ../bin/redis-lab ./cmd/server

dev: cluster-up deploy-all backend-build port-forward
	./bin/redis-lab

port-forward:
	./scripts/port-forward.sh &

teardown: cluster-down
	rm -rf bin/ backend/ui/dist ui/dist ui/node_modules

clean: teardown
```

### `scripts/port-forward.sh`

Chạy 3 port-forward song song để backend connect được vào redis services từ host:
```bash
kubectl port-forward -n redis-standalone svc/redis-standalone-master 6379:6379 &
kubectl port-forward -n redis-sentinel svc/redis-sentinel 6380:6379 &
kubectl port-forward -n redis-sentinel svc/redis-sentinel 26379:26379 &
kubectl port-forward -n redis-cluster svc/redis-cluster 6381:6379 &
wait
```

**Lưu ý**: tên service Bitnami có thể khác — Claude Code phải kiểm tra `kubectl get svc -n <ns>` sau khi deploy và adjust tên service cho đúng.

### `scripts/reset-lab.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
LAB="${1:?usage: reset-lab.sh standalone|sentinel|cluster}"
NS="redis-${LAB}"

helm uninstall "${NS}" -n "${NS}" 2>/dev/null || true
kubectl delete pvc --all -n "${NS}" 2>/dev/null || true
sleep 3
./scripts/deploy-lab.sh "${LAB}"
```

### `README.md`

Ít nhất các section sau (~300 dòng):

1. **What this is** — 1 đoạn giới thiệu + screenshot.
2. **Architecture** — ASCII diagram + bảng so sánh 3 mode.
3. **Prerequisites** — Docker, Kind, kubectl, helm, Go 1.22+, Node 20+, ~4GB RAM trống.
4. **Quick start** — 5 dòng lệnh để chạy được: `make cluster-up && make deploy-all && make backend-build && make port-forward && ./bin/redis-lab`.
5. **What you'll see for each lab** — 3 sub-section, mỗi cái giải thích concept (replication ≠ HA, sentinel failover flow, cluster sharding) và bước demo chính.
6. **Demo scenarios** — 3 kịch bản cụ thể từng bước (có ở phần "Kịch bản end-to-end" bên dưới).
7. **Troubleshooting** — phần này quan trọng:
   - Kind không tạo được cluster (Docker chưa chạy, không đủ RAM)
   - Pod stuck ở Pending (resource không đủ)
   - Sentinel không discover master sau deploy (đợi 30s, hoặc `helm uninstall && deploy lại`)
   - Port-forward bị disconnect (script auto restart)
   - UI không load (check `./bin/redis-lab` log)
8. **Next steps** — gợi ý: refactor thành Operator với kubebuilder, thêm metrics Prometheus, thêm NetworkPolicy partition test.
9. **License & warning** — disclaim rõ: KHÔNG dùng production, backend có quyền cao trên kubeconfig.

## Yêu cầu chất lượng

1. **Idempotent**: mọi script chạy lại nhiều lần không break.
2. **Health check chứ không sleep cứng**: dùng `kubectl wait --for=condition=Ready pod ...` hoặc retry loop với timeout.
3. **Error handling Go**: không `panic(err)` trừ startup main. Mọi error API trả về JSON `{ error: "..." }` với HTTP code phù hợp.
4. **TypeScript strict**: types đầy đủ trong `ui/src/types.ts`, không `any`.
5. **Comment Go code**: mỗi exported function có Go doc. Mỗi file có header 2-3 dòng giải thích chức năng.
6. **Single binary**: sau `make backend-build`, `bin/redis-lab` chạy standalone, UI đã embed.
7. **Console log đẹp**: backend log có timestamp + level, dùng `log/slog` (stdlib).
8. **Demo-friendly**: nút Reset rõ ràng, mỗi lần reset KHÔNG cần down/up cluster, chỉ uninstall + reinstall chart.

## Thứ tự thực hiện bắt buộc

### Phase 1 — Infrastructure
Validate trước khi sang Phase 2:
- [ ] `make cluster-up` tạo Kind cluster thành công.
- [ ] `make deploy-all` deploy 3 chart không lỗi.
- [ ] `kubectl get pods -A | grep redis-` thấy tất cả pod Ready.
- [ ] Test thủ công: `kubectl exec -n redis-sentinel redis-sentinel-node-0 -- redis-cli -p 26379 sentinel master mymaster` trả về thông tin master.
- [ ] Test thủ công: `kubectl exec -n redis-cluster redis-cluster-0 -- redis-cli cluster info` thấy `cluster_state:ok`.

### Phase 2 — Backend
Validate trước khi sang Phase 3:
- [ ] Backend chạy được, log không có error.
- [ ] `curl http://localhost:8080/api/labs/standalone/topology` trả về JSON đúng schema.
- [ ] `curl http://localhost:8080/api/labs/sentinel/topology` thấy currentMaster đúng.
- [ ] `curl http://localhost:8080/api/labs/cluster/topology` thấy 3 shard, mỗi shard có slot ranges.
- [ ] `curl -X POST http://localhost:8080/api/labs/standalone/command -d '{"command":"SET foo bar"}'` trả về `{"result":"OK"}`.
- [ ] `curl -N http://localhost:8080/sse/sentinel` mở stream, sau đó kill master ở terminal khác → thấy event tới trong vòng 10s.

### Phase 3 — UI
Validate trước khi sang Phase 4:
- [ ] `cd ui && npm run dev` chạy được dev server ở port 5173.
- [ ] 3 page render được topology fake data (dùng mock trước khi connect backend).
- [ ] Connect backend qua proxy Vite → topology hiện đúng từ live cluster.
- [ ] Console gõ command → có output.
- [ ] Click "kill master" → thấy topology update real-time.

### Phase 4 — Polish
- [ ] `make backend-build` build single binary thành công.
- [ ] `./bin/redis-lab` standalone chạy, UI accessible ở `http://localhost:8080`.
- [ ] README đầy đủ các section.
- [ ] `make clean` xoá sạch.

## Kịch bản end-to-end phải chạy được (acceptance test)

Sau `make dev`:

### Scenario A: Standalone — Replication is not HA
1. Mở `http://localhost:8080/standalone`.
2. Topology: 1 master + 2 replica, tất cả xanh.
3. Console: `SET hello world` → OK. `GET hello` → "world".
4. Click "Kill master" (pod `redis-standalone-master-0`).
5. Event log: `pod_deleted: redis-standalone-master-0`.
6. Topology: master chuyển sang đỏ.
7. Console: `SET hello world2` → **lỗi** (kết nối không có master). Đây là điểm cốt lõi.
8. Đợi ~30s: StatefulSet recreate pod → master quay lại → SET OK trở lại.
9. **Kết luận in ra UI**: "Replication chỉ là copy data. Không có auto-promote. Đó là lý do bạn cần Sentinel."

### Scenario B: Sentinel — Auto failover
1. Mở `/sentinel`.
2. Topology: 3 redis (1 master xanh + 2 replica xanh) + 3 sentinel (xám).
3. Console: `SET hello world` → OK qua master hiện tại.
4. Click "Kill master".
5. ~5s sau: event log hiện `+sdown master mymaster ...`.
6. Sau đó: `+odown`, rồi `+switch-master mymaster <old-ip> 6379 <new-ip> 6379`.
7. Topology: một replica đổi màu/icon thành master.
8. Console: `GET hello` → "world" (data còn).
9. Console: `SET hello world2` → OK (gửi tới master mới).
10. Pod cũ được StatefulSet recreate → tự join làm replica → topology cập nhật.

### Scenario C: Cluster — Sharding + MOVED redirect
1. Mở `/cluster`.
2. Topology: 6 node (3 master + 3 replica). SlotMap ribbon ở dưới chia 3 màu.
3. Console: `SET user:42 hello` → `slot=N, executed on: redis-cluster-X` (UI hiện rõ đi qua redirect nào).
4. Console: `SET user:{42}:profile alice` và `SET user:{42}:posts 100` → cả 2 cùng slot (hash tag).
5. Console: `MGET user:{42}:profile user:{42}:posts` → OK.
6. Console: `MGET unrelated1 unrelated2` → `CROSSSLOT` error rõ ràng trên UI.
7. Click "Kill" master của shard 1.
8. ~5-10s sau: failover xong, replica cũ thành master mới. SlotMap segment đổi màu sang shard mới.
9. Console: `GET user:42` → vẫn trả về `hello` (data còn).

**Cả 3 scenario phải pass trước khi báo "xong".**

## Pitfall đã biết trước

1. **Sentinel discovery sau deploy mới**: Sentinel cần ~30-60s sau khi pod redis lên để discover master. Backend phải retry connect, không fail hard.
2. **Bitnami chart Service names**: Bitnami đặt tên service không trùng nhau giữa các chart. Chart `bitnami/redis` (sentinel mode) tạo service tên `<release>` cho redis, `<release>-headless`, và port 26379 cho sentinel cùng nằm trong service `<release>`. Chart `bitnami/redis-cluster` tạo service `<release>`. **Claude Code phải `kubectl get svc -n <ns>` sau mỗi deploy để confirm tên thực tế và adjust port-forward script.**
3. **Cluster init mất 1-3 phút**: Bitnami redis-cluster có init job riêng. Backend phải đợi `CLUSTER INFO` trả về `cluster_state:ok` trước khi consider lab ready.
4. **Kill master trong Sentinel mode**: Bitnami chạy redis + sentinel cùng pod, nên kill pod = kill cả redis + sentinel của pod đó. Đây vẫn OK cho demo failover, nhưng note trong UI: "Kill này kết liễu cả redis và sentinel của pod đó."
5. **PVC residue**: helm uninstall không xoá PVC. `reset-lab.sh` phải xoá PVC thủ công.

## Output cuối cùng

Hãy báo cáo từng phase với:
- Danh sách file đã tạo.
- Output của lệnh validation cuối phase.
- Vấn đề gặp phải và cách giải quyết.
- Sau Phase 4: chạy thử cả 3 acceptance scenario và screenshot/log output.

KHÔNG báo "xong" cho đến khi cả 3 scenario pass.
