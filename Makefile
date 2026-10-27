.PHONY: gen
gen: genproto genopenapi

.PHONY: genproto
genproto:
	@./scripts/genproto.sh

.PHONY: genopenapi
genopenapi:
	@./scripts/genopenapi.sh

.PHONY: fmt
fmt:
	goimports -l -w internal/

.PHONY: lint
lint:
	@./scripts/lint.sh

.PHONY: verify
verify: verify-modules verify-order-reliability

.PHONY: verify-modules
verify-modules:
	cd internal/common && go test -count=1 ./...
	cd internal/payment && go test -count=1 ./...
	cd internal/stock && go test -count=1 ./...
	cd internal/kitchen && go test -count=1 ./...

.PHONY: verify-order-reliability
verify-order-reliability:
	cd internal/order && go test -count=1 -race ./...
