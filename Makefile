.PHONY: run fmt vet test test-integration check docker-up docker-down

run:
	go run ./cmd/server

fmt:
	gofmt -w cmd internal tests web

vet:
	go vet -tags=integration ./...

test:
	go test -race ./...

test-integration:
	go test -race -tags=integration -count=1 -timeout=3m ./tests/integration

check: vet test test-integration
	@test -z "$$(gofmt -l cmd internal tests web)" || { echo "Go files need formatting"; exit 1; }
	go build ./...

docker-up:
	docker compose up --build -d --wait

docker-down:
	docker compose down
