# 本地开发便捷命令
# 若 go 不在 PATH（例如使用托管工具链），可用 make run GO=/path/to/go
GO ?= go

.PHONY: run seed seed-reset seed-verify test test-race vet fmt build smoke-llm clean

run:
	$(GO) run ./cmd/server

seed:
	$(GO) run ./cmd/seed

seed-reset:
	$(GO) run ./cmd/seed -reset

# 重建数据并立刻复核规模、场景覆盖与反例拦截
seed-verify: seed-reset
	$(GO) test ./internal/seed -v

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

build:
	mkdir -p bin
	$(GO) build -o bin/server ./cmd/server
	$(GO) build -o bin/seed ./cmd/seed

# 可选：需要 LLM_ENABLED=true 与真实密钥，CI 不执行
smoke-llm:
	$(GO) run ./cmd/smoke

clean:
	rm -rf bin
