.PHONY: cluster-up cluster-down deploy-standalone deploy-sentinel deploy-cluster deploy-all
.PHONY: ui-build backend-build dev clean teardown port-forward validate-phase1

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

port-forward:
	bash ./scripts/port-forward.sh

teardown:
	bash ./scripts/teardown.sh

clean: teardown
	rm -rf bin/ backend/ui/dist ui/dist ui/node_modules
