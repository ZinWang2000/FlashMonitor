# FlashMonitor

固件包体积监控系统

---

## 功能概览

| 功能 | 说明 |
|------|------|
| **批次导入** | 将 CSV 格式的符号大小报告导入数据库，每次构建作为一个独立批次（重复批次名会被拒绝） |
| **批次列表** | 查询某个包下已存储的所有批次名称 |
| **批次导出** | 将指定批次的数据导出为 CSV，按 Size 降序排列 |
| **批次删除** | 删除指定批次的全部数据行 |
| **全量对比** | 对比两个批次中所有文件的大小差异，输出 CSV |
| **TopN 对比** | 对比变化最大的前 N 个文件，支持输出 CSV 或 ECharts 可视化 HTML |
| **时序分析** | 跨多个批次追踪指定文件的大小变化趋势，输出 CSV |
| **外部对比** | 将数据库中的批次与外部 CSV 文件做全外连接对比 |

---

## 快速开始

### 启动 HTTP 服务器

```bash
flashmonitor.exe --port 8080 --db ./data/myproject.db
```

浏览器访问 `http://localhost:8080`，通过网页界面完成所有操作。

### MCP stdio 模式（供 AI 客户端调用）

```bash
flashmonitor.exe --stdio --db ./data/myproject.db
```

从 stdin 读取 JSON-RPC 请求，向 stdout 写入响应，兼容 MCP 协议。

### 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--port` | `8080` | HTTP 服务端口 |
| `--db` | `flashmonitor.db` | DuckDB 数据库文件路径 |
| `--stdio` | — | 启用 MCP stdio 模式（与 `--port` 互斥） |
| `--version` | — | 显示版本号并退出 |

> **注意**：`--db` 的所在目录即为输出文件的根目录，所有报告文件均写入该目录。

---

## 输入 CSV 格式

导入的 CSV 文件须包含以下列（顺序不限，标题行必须存在）：

| 列名 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `FileName` | 字符串 | **是** | 目标文件名（如 `foo.o`），用作唯一键 |
| `Size` | 整数 | **是** | 文件大小（字节） |
| `Section` | 字符串 | 否 | 所属段（如 `.text`、`.data`） |
| `ModuleName` | 字符串 | 否 | 所属模块 |
| `MangledName` | 字符串 | 否 | C++ 修饰名 |

示例：

```csv
Section,ModuleName,FileName,MangledName,Size
.text,libfoo,foo.o,_ZN3Foo3barEv,1024
.data,libbar,bar.o,_ZN3Bar4dataEv,256
```

---

## JSON-RPC API

所有功能均通过 `POST /rpc` 端点以 JSON-RPC 2.0 协议调用。

### flash.import — 导入 CSV

```json
{
  "jsonrpc": "2.0", "id": 1,
  "method": "flash.import",
  "params": {
    "csvPath": "E:/builds/v1.2.3/sizes.csv",
    "packageName": "myproject",
    "tableName": "v1.2.3"
  }
}
```

- `tableName` 可选，默认使用当前时间戳 `yymmddhhmmss`
- 返回：`{ "rowsInserted": 1234, "tableName": "v1.2.3" }`
- **若 `tableName` 已存在则返回错误**，需先调用 `flash.deleteTable` 删除后再导入

### flash.listTables — 列出批次

```json
{
  "jsonrpc": "2.0", "id": 2,
  "method": "flash.listTables",
  "params": { "packageName": "myproject" }
}
```

- 返回：`{ "tables": ["v1.0.0", "v1.1.0", "v1.2.3"] }`
- 包不存在时自动创建，返回空数组

### flash.compareAll — 全量对比

```json
{
  "jsonrpc": "2.0", "id": 3,
  "method": "flash.compareAll",
  "params": {
    "packageName": "myproject",
    "tableName1": "v1.1.0",
    "tableName2": "v1.2.3",
    "outputPath": "E:/reports/compare.csv"
  }
}
```

- `outputPath` 可选，默认自动生成文件名
- 输出 CSV 列：`Section, ModuleName, FileName, MangledName, Size1, Size2, dSize`
- `dSize = Size2 - Size1`（正值表示增大）

### flash.compareTopN — TopN 对比

```json
{
  "jsonrpc": "2.0", "id": 4,
  "method": "flash.compareTopN",
  "params": {
    "packageName": "myproject",
    "tableName1": "v1.1.0",
    "tableName2": "v1.2.3",
    "topN": 20,
    "outputFormat": "html"
  }
}
```

- `outputFormat`：`"csv"`（默认）或 `"html"`（ECharts 水平柱状图）
- HTML 输出要求 `topN > 0`

### flash.timeSeries — 时序分析

```json
{
  "jsonrpc": "2.0", "id": 5,
  "method": "flash.timeSeries",
  "params": {
    "packageName": "myproject",
    "tableNames": ["v1.0.0", "v1.1.0", "v1.2.3"],
    "filePattern": ".*foo.*"
  }
}
```

- `filePattern` 可选，为正则表达式，过滤 `FileName`
- 输出 CSV：每个批次名作为一列，末尾附 `dSize`（最后批次 − 第一批次）

### flash.compareExternal — 外部 CSV 对比

```json
{
  "jsonrpc": "2.0", "id": 6,
  "method": "flash.compareExternal",
  "params": {
    "packageName": "myproject",
    "tableName": "v1.2.3",
    "externalCsvPath": "E:/reference/baseline.csv",
    "topN": 50,
    "outputFormat": "csv"
  }
}
```

- 将数据库批次与本地外部 CSV 做 FULL OUTER JOIN 对比
- `topN` 可选，为 0 或省略时返回全部行

### flash.exportTable — 导出批次

```json
{
  "jsonrpc": "2.0", "id": 8,
  "method": "flash.exportTable",
  "params": {
    "packageName": "myproject",
    "tableName": "v1.2.3",
    "outputPath": "E:/reports/export_v1.2.3.csv"
  }
}
```

- `outputPath` 可选，默认自动生成文件名 `export_{packageName}_{tableName}.csv`
- 输出 CSV 列：`Section, ModuleName, FileName, MangledName, Size`，按 **Size 降序**排列
- 返回：`{ "rows": 1024, "outputPath": "..." }`

### flash.deleteTable — 删除批次

```json
{
  "jsonrpc": "2.0", "id": 9,
  "method": "flash.deleteTable",
  "params": {
    "packageName": "myproject",
    "tableName": "v1.2.3"
  }
}
```

- 删除指定批次的全部数据行，不删除物理表结构
- 批次不存在时 `rowsDeleted` 为 0，不报错
- 返回：`{ "rowsDeleted": 1024 }`

### flash.getLogs — 获取日志

```json
{
  "jsonrpc": "2.0", "id": 7,
  "method": "flash.getLogs",
  "params": { "last": 50 }
}
```

---

## MCP 工具集成

FlashMonitor 完全兼容 **Model Context Protocol (MCP)**，可作为工具服务器供 Claude 等 AI 客户端直接调用。

支持的协议方法：

- `initialize` — MCP 握手，返回协议版本 `2024-11-05`
- `tools/list` — 返回所有工具的 JSON Schema 描述
- `tools/call` — 调用指定工具（映射到对应 `flash.*` 方法）

工具名称（MCP 格式，使用下划线）：

`flash_import` / `flash_listTables` / `flash_exportTable` / `flash_deleteTable` / `flash_compareAll` / `flash_compareTopN` / `flash_timeSeries` / `flash_compareExternal` / `flash_getLogs`

---

## 网页界面

启动 HTTP 模式后访问根路径即可使用内置前端，包含以下功能卡片：

- **导入用量数据** — 文件上传表单，重复批次名会被拒绝并提示错误
- **对比所有文件** — 全量对比两个批次，输出 CSV
- **对比 TopN** — 对比变化最大的前 N 个文件，支持 CSV / HTML
- **文件大小历史变化** — 时序分析，支持多批次标签输入
- **对比本地用量** — 与服务器端外部 CSV 文件对比
- **导出用量表** — 将指定批次导出为按 Size 降序的 CSV
- **删除用量表** — 删除指定批次数据（二次确认后执行）
- **System Logs** — 实时查看最近操作日志

左上角汉堡菜单（☰）打开侧边栏，显示数据库中所有包和批次；点击批次标签可自动填入所有功能卡片的输入框。

> CSV 文件上传走 `POST /upload` 接口，文件在服务器端临时保存后立即导入并删除。

---

## 项目结构

```
FlashMonitor/
├── cmd/flashmonitor/main.go          # 程序入口
├── internal/
│   ├── db/                           # DuckDB 数据访问层
│   │   ├── db.go                     # 连接管理、建表
│   │   ├── importer.go               # CSV 批量导入（含重复批次检测）
│   │   ├── query.go                  # 批次列表查询
│   │   ├── export.go                 # 批次数据导出
│   │   ├── delete.go                 # 批次数据删除
│   │   ├── compare.go                # 双批次对比
│   │   ├── timeseries.go             # 时序分析
│   │   └── external.go              # 外部 CSV 对比
│   ├── validator/validator.go        # 名称白名单 + 路径防穿越
│   ├── logger/ring_logger.go         # 环形缓冲日志 + 日志文件滚动
│   ├── report/
│   │   ├── csv_writer.go             # CSV 报告生成
│   │   └── html_writer.go           # ECharts HTML 报告生成
│   ├── rpc/
│   │   ├── handler.go                # JSON-RPC 2.0 分发器
│   │   └── methods.go               # 各方法实现
│   └── transport/
│       ├── stdio.go                  # MCP stdio 传输层
│       └── http_sse.go              # HTTP + SSE 传输层
├── web/
│   ├── embed.go                      # go:embed 内嵌前端资源
│   └── index.html                   # 单页前端（纯 HTML + JS）
├── libs/
│   ├── seekpos_fix.s                 # GCC 15 ABI 兼容汇编补丁
│   └── libseekpos_fix.a             # 编译后的补丁静态库
├── test/
│   ├── integration_test.go           # 全流程集成测试
│   └── gen_csv/main.go              # 百万行测试 CSV 生成工具
├── Makefile
└── go.mod
```

---

## 构建

### 环境要求

| 工具 | 版本 | 说明 |
|------|------|------|
| Go | ≥ 1.24 | 需在系统 PATH 中可见 |
| GCC (MinGW-w64) | 与 go-duckdb 兼容版本 | 将 `C:\mingw64\bin` 加入系统 PATH |
| CGO | 已启用 | DuckDB 驱动依赖 CGO |

### 构建命令

将 `C:\mingw64\bin` 加入系统环境变量 `PATH` 后，在项目根目录执行：

```bash
go build -o flashmonitor.exe ./cmd/flashmonitor
```

GoLand 等 IDE 的内置终端会自动继承系统 PATH，无需额外配置。

---

## 安全设计

- **名称白名单**：`packageName` 和 `tableName` 仅允许 `^[a-zA-Z0-9_\-.]+$`，最大长度 128，防止 SQL 注入
- **重复导入保护**：导入前检查批次名是否已存在，存在则拒绝并返回错误，避免数据污染
- **SQL 标识符**：表名以双引号括起，不可通过用户输入篡改 SQL 结构
- **路径防穿越**：输出路径必须在 `--db` 文件所在目录内，拒绝 `../` 等跳出操作
- **CSV 路径**：`read_csv_auto` 不支持参数化路径，使用单引号转义后嵌入 SQL 字面量
- **单连接串行写入**：`MaxOpenConns=1`，避免 DuckDB 并发写入冲突

---

## 日志

运行期间日志同时写入：

- **内存环形缓冲区**：最近 1000 条，通过 `flash.getLogs` 查询
- **日志文件**：`<db目录>/logs/flashmonitor_YYYYMMDD.log`，每日自动滚动，JSON Lines 格式

---

## 测试工具

###  CSV 生成器

```bash
go run ./test/gen_csv/main.go \
  -rows 1000000 \
  -files 5000 \
  -packages 3 \
  -out ./testdata \
  -seed 42
```

参数说明：

| 参数 | 说明                         |
|------|----------------------------|
| `-rows` | 每个 CSV 的总行数                |
| `-files` | 文件名池大小（distinct FileName 数量） |
| `-packages` | 生成的包数量（每包一个 CSV）           |
| `-out` | 输出目录                       |
| `-seed` | 行数据随机种子                    |
| `-fseed` | 文件名池随机种子；`0`（默认）= 与 `-seed` 相同 |

`-fseed` 用于在多次独立运行之间保持文件名集合一致，同时允许行数据使用不同种子：

```bash
# 两次独立运行，FileName 集合完全相同，但 Size/Section 等数据不同
go run ./test/gen_csv/main.go -seed 1 -fseed 77 -files 5000 -rows 200000 -out ./v1
go run ./test/gen_csv/main.go -seed 2 -fseed 77 -files 5000 -rows 200000 -out ./v2
```
