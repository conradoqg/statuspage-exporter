IMAGE ?= ghcr.io/$(shell echo $$(git remote get-url origin 2>/dev/null | sed -E 's#.*github.com[:/](.+)\.git#\1#'))/statuspage-exporter:latest
BIN   ?= output/statuspage-exporter

.PHONY: build build-linux run test fmt vet docker-build docker-run docker-push clean

build:
	@echo "Building $(BIN)..."
	@mkdir -p output
	@GOCACHE=$$PWD/.gocache go build -o $(BIN) ./cmd/statuspage-exporter

build-linux:
	@echo "Building linux/amd64 binary..."
	@mkdir -p output
	@GOCACHE=$$PWD/.gocache CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BIN)-linux-amd64 ./cmd/statuspage-exporter

run: build
	@./$(BIN) --config=config.yaml --listen=:9090

test:
	@GOCACHE=$$PWD/.gocache go test ./...

fmt:
	@go fmt ./...

vet:
	@go vet ./...

docker-build:
	@echo "Building Docker image $(IMAGE)..."
	@docker build -t $(IMAGE) .

docker-run:
	@docker run --rm -p 8080:8080 $(IMAGE)

docker-push:
	@docker push $(IMAGE)

clean:
	@rm -rf output .gocache

