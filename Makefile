.PHONY: docker-build compose-up compose-down cluster-up cluster-scale cluster-down loadtest-build loadtest-qps loadtest-tpm loadtest-ramp seed seed-reset web-install web-dev web-build run build test vet lint tidy migrate-up migrate-down migrate-version dev-up dev-down

build:
	go build -o bin/gateway ./cmd/gateway
	go build -o bin/migrate ./cmd/migrate

run:
	go run ./cmd/gateway -config configs/config.yaml

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

migrate-up:
	go run ./cmd/migrate -config configs/config.yaml up

migrate-down:
	go run ./cmd/migrate -config configs/config.yaml down 1

migrate-version:
	go run ./cmd/migrate -config configs/config.yaml version

dev-up:
	docker compose -f deployments/docker-compose.yml up -d

dev-down:
	docker compose -f deployments/docker-compose.yml down

web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

seed:
	go run ./cmd/seed -config configs/config.yaml

seed-reset:
	go run ./cmd/seed -config configs/config.yaml -reset

loadtest-build:
	go build -o bin/loadtest ./loadtest

# demo scenarios (need `make seed` data and a running gateway)
loadtest-qps: loadtest-build
	bin/loadtest -key sk-demo-a2-customer-test -model doubao-flash -qps 10 -duration 10s -expect-qps 2

loadtest-tpm: loadtest-build
	bin/loadtest -key sk-demo-a2-customer-test -model doubao-flash -mode tpm -tpm 60000 -duration 130s -expect-tpm 20000

loadtest-ramp: loadtest-build
	bin/loadtest -key sk-demo-a1-customer-service -model doubao-flash -mode ramp -ramp-start 20 -ramp-step 20 -ramp-max 200 -ramp-interval 5s -stop-ratio 0.3

# ---- deployment (docs/deployment.md) ----
IMAGE ?= llm-gateway:latest
SLAVES ?= 3

docker-build:
	docker build -f deployments/Dockerfile -t $(IMAGE) .

compose-up:
	cd deployments/docker && docker compose up -d --build

compose-down:
	cd deployments/docker && docker compose down

cluster-up:
	cd deployments/cluster && docker compose up -d --build

cluster-scale:
	cd deployments/cluster && docker compose up -d --scale gateway-slave=$(SLAVES)

cluster-down:
	cd deployments/cluster && docker compose down
