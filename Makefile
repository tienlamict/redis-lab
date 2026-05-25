.PHONY: help cluster-up cluster-down deploy-standalone deploy-sentinel deploy-cluster deploy-all
.PHONY: reset-standalone reset-sentinel reset-cluster validate-phase1
.PHONY: ui-install ui-build backend-build dev port-forward port-forward-fg teardown clean

help:
	@echo "Targets:"
	@echo "  cluster-up        - Create the Kind cluster (1 control-plane + 3 worker)"
	@echo "  deploy-all        - Deploy standalone + sentinel + cluster via Bitnami helm charts"
	@echo "  ui-build          - npm install + vite build + copy dist to backend/ui/dist (for embed)"
	@echo "  backend-build     - Build single-binary bin/redis-lab(.exe) with UI embedded"
	@echo "  port-forward      - Forward redis services to localhost (background)"
	@echo "  dev               - Full bring-up: cluster + deploy + build + pf + run"
	@echo "  reset-standalone  - helm uninstall + PVC wipe + reinstall (per lab)"
	@echo "  teardown          - Uninstall releases + delete Kind cluster"
	@echo "  clean             - teardown + remove build artifacts"

cluster-up:
	kind create cluster --config kind/cluster.yaml

cluster-down:
	kind delete cluster --name redis-lab

deploy-standalone:
	bash ./scripts/deploy-lab.sh standalone

deploy-sentinel:
	bash ./scripts/deploy-lab.sh sentinel

deploy-cluster:
	bash ./scripts/deploy-lab.sh cluster

deploy-all: deploy-standalone deploy-sentinel deploy-cluster

reset-standalone:
	bash ./scripts/reset-lab.sh standalone

reset-sentinel:
	bash ./scripts/reset-lab.sh sentinel

reset-cluster:
	bash ./scripts/reset-lab.sh cluster

validate-phase1:
	@echo "=== All redis-* pods ==="
	@kubectl get pods -A | grep '^redis-' || true
	@echo
	@echo "=== Sentinel master info ==="
	@kubectl exec -n redis-sentinel redis-sentinel-node-0 -c sentinel -- redis-cli -p 26379 sentinel master mymaster || true
	@echo
	@echo "=== Cluster info ==="
	@kubectl exec -n redis-cluster redis-cluster-0 -- redis-cli cluster info || true

ui-install:
	cd ui && npm install

ui-build: ui-install
	cd ui && npm run build
	rm -rf backend/ui/dist
	mkdir -p backend/ui
	cp -r ui/dist backend/ui/dist

backend-build: ui-build
	mkdir -p bin
	cd backend && go build -o ../bin/redis-lab$(EXE) ./cmd/server

# `make dev` is the one-shot bring-up. After it returns, the binary is in
# foreground; ports stay open while it runs. Ctrl-C to stop.
dev: cluster-up deploy-all backend-build port-forward
	./bin/redis-lab$(EXE)

# port-forward target backgrounds the script so the chain `make port-forward &&
# ./bin/redis-lab` doesn't hang. For an interactive foreground session, run
# `bash scripts/port-forward.sh` directly.
port-forward:
	bash ./scripts/port-forward.sh &

# port-forward-fg keeps the script in foreground; ctrl-c stops all forwards.
port-forward-fg:
	bash ./scripts/port-forward.sh

teardown:
	bash ./scripts/teardown.sh

clean: teardown
	rm -rf bin/ backend/ui/dist ui/dist ui/node_modules
