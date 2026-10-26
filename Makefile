# 本地开发便捷命令
# 若 go 不在 PATH（例如使用托管工具链），可用 make run GO=/path/to/go
GO ?= go

.PHONY: run seed seed-reset seed-verify test test-race vet fmt build smoke-llm clean kb-import kb-import-dry kb-publish im-prep scenarios

# 美团门店 IM 导出 → testdata/im_export/candidates.jsonl + stats.json
# 这是 P0/P1/P2 的唯一输入口；原始 txt 含手机号与地址，只写脱敏文本
IM_ROOT ?= ../JDDC-Baseline-Seq2Seq/数据/duty-im-export
im-prep:
	$(GO) run ./cmd/imprep -src "$(IM_ROOT)" -out testdata/im_export

# 刷新评测元数据：生成器场景 + 手写 IM 场景（testdata/im_export/scenarios_im.jsonl）
# 不碰数据库，库已存在时也能跑
scenarios:
	$(GO) run ./cmd/scenarios -dir testdata

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

# 京东帮助中心 FAQ（docs/jd-faq/faq.jsonl）→ knowledge_articles
# 默认写 draft；核对无误后 make kb-publish 才进入 AI 检索
kb-import:
	$(GO) run ./cmd/kbimport

kb-import-dry:
	$(GO) run ./cmd/kbimport -dry-run

kb-publish:
	$(GO) run ./cmd/kbimport -status published

# 可选：需要 LLM_ENABLED=true 与真实密钥，CI 不执行
smoke-llm:
	$(GO) run ./cmd/smoke

clean:
	rm -rf bin
