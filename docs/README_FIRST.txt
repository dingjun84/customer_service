电商 AI 客服 MVP Agent 交接包

建议先阅读：ecommerce_ai_customer_service_mvp_spec.docx
便于 Agent/代码模型解析的 Markdown 版本：ecommerce_ai_customer_service_mvp_spec.md
可独立执行的 SQLite schema（包含 PRAGMA）：ecommerce_support_mvp_sqlite.sql
DDL-only migration 参考文件（不含连接级 PRAGMA）：ecommerce_support_mvp_migration_0001.sql

实施说明：
1. 真正项目中的 migrations/0001_init.sql 以 DDL-only 文件为起点。
2. 数据库启动阶段应在事务外配置 foreign_keys、busy_timeout 和 WAL；foreign_keys/busy_timeout 应确保每个连接生效。
3. SQL 文件不含 seed/test 数据；测试数据应由 go run ./cmd/seed 单独创建。
4. 按规格书中的 Phase 0-7 顺序实施。建议先冻结公共 DTO / service interfaces，再分派多个 Agent 并行开发。
