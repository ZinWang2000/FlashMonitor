package rpc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"flashmonitor/internal/db"
	"flashmonitor/internal/logger"
	"flashmonitor/internal/report"
	"flashmonitor/internal/validator"
)

type Methods struct {
	db      *db.DB
	baseDir string
}

// NewMethods 创建 Methods 实例，绑定数据库连接和报告输出根目录。
//
// 输入:
//   - database: 已打开的 DuckDB 连接
//   - baseDir:  输出文件根目录（所有报告须在此目录内）
func NewMethods(database *db.DB, baseDir string) *Methods {
	return &Methods{db: database, baseDir: baseDir}
}

func (m *Methods) Initialize(req Request) Response {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "flashmonitor",
			"version": "1.0.0",
		},
	}
	return NewResultResponse(req.ID, result)
}

func (m *Methods) ToolsList(req Request) Response {
	tools := []map[string]interface{}{
		mcpTool("flash_import", "Import a CSV file into the database",
			map[string]interface{}{
				"csvPath":     toolProp("string", "Absolute path to the CSV file"),
				"packageName": toolProp("string", "Package identifier"),
				"tableName":   toolProp("string", "Batch label (optional, defaults to timestamp)"),
			}, []string{"csvPath", "packageName"}),
		mcpTool("flash_listTables", "List all batch names for a package",
			map[string]interface{}{
				"packageName": toolProp("string", "Package identifier"),
			}, []string{"packageName"}),
		mcpTool("flash_compareAll", "Compare all files between two batches",
			map[string]interface{}{
				"packageName": toolProp("string", "Package identifier"),
				"tableName1":  toolProp("string", "First batch name"),
				"tableName2":  toolProp("string", "Second batch name"),
				"outputPath":  toolProp("string", "Output CSV path (optional)"),
			}, []string{"packageName", "tableName1", "tableName2"}),
		mcpTool("flash_compareTopN", "Compare top N files by size change",
			map[string]interface{}{
				"packageName":  toolProp("string", "Package identifier"),
				"tableName1":   toolProp("string", "First batch name"),
				"tableName2":   toolProp("string", "Second batch name"),
				"topN":         toolProp("integer", "Number of top results"),
				"outputFormat": toolProp("string", "Output format: csv or html"),
				"outputPath":   toolProp("string", "Output file path (optional)"),
			}, []string{"packageName", "tableName1", "tableName2", "topN"}),
		mcpTool("flash_timeSeries", "Time-series analysis across multiple batches",
			map[string]interface{}{
				"packageName":  toolProp("string", "Package identifier"),
				"filePattern":  toolProp("string", "Regex pattern to filter filenames (optional)"),
				"tableNames":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Ordered list of batch names"},
				"outputFormat": toolProp("string", "Output format: csv"),
				"outputPath":   toolProp("string", "Output file path (optional)"),
			}, []string{"packageName", "tableNames"}),
		mcpTool("flash_compareExternal", "Compare a batch against an external CSV",
			map[string]interface{}{
				"packageName":     toolProp("string", "Package identifier"),
				"tableName":       toolProp("string", "Batch name in DB"),
				"externalCsvPath": toolProp("string", "Absolute path to external CSV"),
				"topN":            toolProp("integer", "Limit results (optional)"),
				"outputFormat":    toolProp("string", "Output format: csv or html"),
				"outputPath":      toolProp("string", "Output file path (optional)"),
			}, []string{"packageName", "tableName", "externalCsvPath"}),
		mcpTool("flash_exportTable", "Export a batch as CSV sorted by size descending",
			map[string]interface{}{
				"packageName": toolProp("string", "Package identifier"),
				"tableName":   toolProp("string", "Batch name to export"),
				"outputPath":  toolProp("string", "Output CSV path (optional)"),
			}, []string{"packageName", "tableName"}),
		mcpTool("flash_deleteTable", "Delete all rows of a batch",
			map[string]interface{}{
				"packageName": toolProp("string", "Package identifier"),
				"tableName":   toolProp("string", "Batch name to delete"),
			}, []string{"packageName", "tableName"}),
		mcpTool("flash_getLogs", "Get recent log entries",
			map[string]interface{}{
				"last": toolProp("integer", "Number of recent entries to return"),
			}, []string{}),
	}
	return NewResultResponse(req.ID, map[string]interface{}{"tools": tools})
}

func mcpTool(name, desc string, props map[string]interface{}, required []string) map[string]interface{} {
	return map[string]interface{}{
		"name":        name,
		"description": desc,
		"inputSchema": map[string]interface{}{
			"type":       "object",
			"properties": props,
			"required":   required,
		},
	}
}

func toolProp(typ, desc string) map[string]interface{} {
	return map[string]interface{}{"type": typ, "description": desc}
}

func (m *Methods) ToolsCall(req Request) Response {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, "tools/call: "+err.Error())
	}

	methodName := ""
	switch params.Name {
	case "flash_import":
		methodName = "flash.import"
	case "flash_listTables":
		methodName = "flash.listTables"
	case "flash_compareAll":
		methodName = "flash.compareAll"
	case "flash_compareTopN":
		methodName = "flash.compareTopN"
	case "flash_timeSeries":
		methodName = "flash.timeSeries"
	case "flash_compareExternal":
		methodName = "flash.compareExternal"
	case "flash_exportTable":
		methodName = "flash.exportTable"
	case "flash_deleteTable":
		methodName = "flash.deleteTable"
	case "flash_getLogs":
		methodName = "flash.getLogs"
	default:
		return NewErrorResponse(req.ID, ErrCodeMethodNotFound, "unknown tool: "+params.Name)
	}

	inner := Request{
		JSONRPC: "2.0",
		Method:  methodName,
		Params:  params.Arguments,
		ID:      req.ID,
	}
	h := NewHandler(m)
	resp := h.Handle(mustMarshal(inner))

	if resp.Error != nil {
		return resp
	}
	content := []map[string]interface{}{
		{"type": "text", "text": mustMarshalString(resp.Result)},
	}
	return NewResultResponse(req.ID, map[string]interface{}{"content": content})
}

// Import 处理 flash.import 请求，将 CSV 文件导入到指定包的指定批次。
//
// 输入（JSON 字段）:
//   - csvPath:     CSV 文件的绝对路径（必填）
//   - packageName: 目标包名（必填，需通过白名单校验）
//   - tableName:   批次标签（可选，省略时自动生成时间戳 yymmddhhmmss）
//
// 输出: { packageName, tableName, rowsInserted }
type importParams struct {
	CSVPath     string `json:"csvPath"`
	PackageName string `json:"packageName"`
	TableName   string `json:"tableName,omitempty"`
}

func (m *Methods) Import(req Request) Response {
	var p importParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}
	if p.CSVPath == "" {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, "csvPath is required")
	}
	if err := validator.ValidateName(p.PackageName); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, "packageName: "+err.Error())
	}

	tableName := p.TableName
	if tableName == "" {
		tableName = time.Now().Format("060102150405")
	} else {
		if err := validator.ValidateName(tableName); err != nil {
			return NewErrorResponse(req.ID, ErrCodeInvalidParams, "tableName: "+err.Error())
		}
	}

	absCSV, err := filepath.Abs(p.CSVPath)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, "csvPath: "+err.Error())
	}
	if _, err := os.Stat(absCSV); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, "csvPath not found: "+absCSV)
	}

	logger.Infof("import: package=%q table=%q csv=%q", p.PackageName, tableName, absCSV)

	result, err := m.db.ImportCSV(p.PackageName, tableName, absCSV)
	if err != nil {
		logger.Errorf("import failed: %v", err)
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}

	logger.Infof("import done: %d rows inserted", result.RowsInserted)
	return NewResultResponse(req.ID, map[string]interface{}{
		"packageName":  result.PackageName,
		"tableName":    result.TableName,
		"rowsInserted": result.RowsInserted,
	})
}

// ListTables 处理 flash.listTables 请求，返回指定包内所有批次名称。
//
// 输入（JSON 字段）:
//   - packageName: 包名（必填）
//
// 输出: { packageName, tables: [...] }
//
// 注意事项:
//   - 包不存在时自动创建表，返回空数组
type listTablesParams struct {
	PackageName string `json:"packageName"`
}

func (m *Methods) ListTables(req Request) Response {
	var p listTablesParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}
	if err := validator.ValidateName(p.PackageName); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, "packageName: "+err.Error())
	}

	tables, err := m.db.ListTables(p.PackageName)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}

	return NewResultResponse(req.ID, map[string]interface{}{
		"packageName": p.PackageName,
		"tables":      tables,
	})
}

// ListAllPackages 处理 flash.listAllPackages 请求，返回数据库中所有包及其批次列表。
//
// 输出: { packages: [{ name, tables: [...] }, ...] }
//
// 注意事项:
//   - 主要供前端侧边栏使用；MCP 调用请使用 flash.listTables
func (m *Methods) ListAllPackages(req Request) Response {
	packages, err := m.db.ListAllPackagesWithTables()
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}
	if packages == nil {
		packages = []db.PackageInfo{}
	}
	return NewResultResponse(req.ID, map[string]interface{}{
		"packages": packages,
	})
}

// CompareAll 处理 flash.compareAll 请求，对比两个批次的全量文件大小差异，输出 CSV。
//
// 输入（JSON 字段）:
//   - packageName: 包名（必填）
//   - tableName1:  基线批次（必填）
//   - tableName2:  对比批次（必填）
//   - outputPath:  输出文件路径（可选，须在 baseDir 内）
//
// 输出: { rows, outputPath }
type compareAllParams struct {
	PackageName string `json:"packageName"`
	TableName1  string `json:"tableName1"`
	TableName2  string `json:"tableName2"`
	OutputPath  string `json:"outputPath,omitempty"`
}

func (m *Methods) CompareAll(req Request) Response {
	var p compareAllParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}
	if err := validateNames(p.PackageName, p.TableName1, p.TableName2); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}

	outPath, err := validator.ValidateOutputPath(p.OutputPath, m.baseDir)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}
	if p.OutputPath == "" {
		outPath = filepath.Join(m.baseDir, fmt.Sprintf("compare_%s_%s_vs_%s.csv",
			p.PackageName, p.TableName1, p.TableName2))
	}

	rows, err := m.db.CompareAll(p.PackageName, p.TableName1, p.TableName2)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}

	if err := report.WriteCompareCSV(rows, outPath); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}

	logger.Infof("compareAll: %d rows → %s", len(rows), outPath)
	return NewResultResponse(req.ID, map[string]interface{}{
		"rows":       len(rows),
		"outputPath": outPath,
	})
}

// CompareTopN 处理 flash.compareTopN 请求，返回变化最大的前 N 个文件，支持 CSV/HTML 输出。
//
// 输入（JSON 字段）:
//   - topN:         返回行数（必填，>0）
//   - outputFormat: "csv"（默认）或 "html"（ECharts 可视化）
//   - outputPath:   输出文件路径（可选）
//
// 输出: { rows, outputPath, format }
type compareTopNParams struct {
	PackageName  string `json:"packageName"`
	TableName1   string `json:"tableName1"`
	TableName2   string `json:"tableName2"`
	TopN         int    `json:"topN"`
	OutputFormat string `json:"outputFormat,omitempty"`
	OutputPath   string `json:"outputPath,omitempty"`
}

func (m *Methods) CompareTopN(req Request) Response {
	var p compareTopNParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}
	if err := validateNames(p.PackageName, p.TableName1, p.TableName2); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}
	if p.TopN <= 0 {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, "topN must be > 0")
	}
	if p.OutputFormat == "html" && p.TopN <= 0 {
		return NewErrorResponse(req.ID, ErrCodeHTMLNeedsTopN, "HTML output requires topN > 0")
	}

	outPath, err := validator.ValidateOutputPath(p.OutputPath, m.baseDir)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}

	rows, err := m.db.CompareTopN(p.PackageName, p.TableName1, p.TableName2, p.TopN)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}

	ext := ".csv"
	if p.OutputFormat == "html" {
		ext = ".html"
	}
	if p.OutputPath == "" {
		outPath = filepath.Join(m.baseDir, fmt.Sprintf("topN%d_%s_%s_vs_%s%s",
			p.TopN, p.PackageName, p.TableName1, p.TableName2, ext))
	}

	if p.OutputFormat == "html" {
		err = report.WriteCompareHTML(rows, p.TableName1, p.TableName2, outPath)
	} else {
		err = report.WriteCompareCSV(rows, outPath)
	}
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}

	logger.Infof("compareTopN: %d rows → %s", len(rows), outPath)
	return NewResultResponse(req.ID, map[string]interface{}{
		"rows":       len(rows),
		"outputPath": outPath,
		"format":     p.OutputFormat,
	})
}

// TimeSeries 处理 flash.timeSeries 请求，跨多批次追踪文件大小变化趋势，输出 CSV。
//
// 输入（JSON 字段）:
//   - packageName: 包名（必填）
//   - tableNames:  有序批次名列表（必填）
//   - filePattern: 正则过滤 FileName（可选）
//   - outputPath:  输出文件路径（可选）
//
// 输出: { rows, outputPath }
type timeSeriesParams struct {
	PackageName  string   `json:"packageName"`
	FilePattern  string   `json:"filePattern,omitempty"`
	TableNames   []string `json:"tableNames"`
	OutputFormat string   `json:"outputFormat,omitempty"`
	OutputPath   string   `json:"outputPath,omitempty"`
}

func (m *Methods) TimeSeries(req Request) Response {
	var p timeSeriesParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}
	if err := validator.ValidateName(p.PackageName); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, "packageName: "+err.Error())
	}
	if len(p.TableNames) == 0 {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, "tableNames must not be empty")
	}
	for _, tn := range p.TableNames {
		if err := validator.ValidateName(tn); err != nil {
			return NewErrorResponse(req.ID, ErrCodeInvalidParams, fmt.Sprintf("tableName %q: %v", tn, err))
		}
	}

	outPath, err := validator.ValidateOutputPath(p.OutputPath, m.baseDir)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}
	if p.OutputPath == "" {
		outPath = filepath.Join(m.baseDir, fmt.Sprintf("timeseries_%s.csv", p.PackageName))
	}

	rows, err := m.db.TimeSeries(p.PackageName, p.FilePattern, p.TableNames)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}

	if err := report.WriteTimeSeriesCSV(rows, p.TableNames, outPath); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}

	logger.Infof("timeSeries: %d rows → %s", len(rows), outPath)
	return NewResultResponse(req.ID, map[string]interface{}{
		"rows":       len(rows),
		"outputPath": outPath,
	})
}

// CompareExternal 处理 flash.compareExternal 请求，将数据库批次与外部 CSV 文件对比。
//
// 输入（JSON 字段）:
//   - packageName:     包名（必填）
//   - tableName:       数据库批次名（必填）
//   - externalCsvPath: 外部 CSV 文件的服务器端绝对路径（必填）
//   - topN:            返回行数上限（可选，0=全部）
//   - outputFormat:    "csv" 或 "html"（可选）
//   - outputPath:      输出文件路径（可选）
//
// 输出: { rows, outputPath, format }
type compareExternalParams struct {
	PackageName     string `json:"packageName"`
	TableName       string `json:"tableName"`
	ExternalCsvPath string `json:"externalCsvPath"`
	TopN            int    `json:"topN,omitempty"`
	OutputFormat    string `json:"outputFormat,omitempty"`
	OutputPath      string `json:"outputPath,omitempty"`
}

func (m *Methods) CompareExternal(req Request) Response {
	var p compareExternalParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}
	if err := validateNames(p.PackageName, p.TableName); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}
	if p.ExternalCsvPath == "" {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, "externalCsvPath is required")
	}
	if p.OutputFormat == "html" && p.TopN <= 0 {
		return NewErrorResponse(req.ID, ErrCodeHTMLNeedsTopN, "HTML output requires topN > 0")
	}

	absCSV, err := filepath.Abs(p.ExternalCsvPath)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, "externalCsvPath: "+err.Error())
	}
	if _, err := os.Stat(absCSV); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, "externalCsvPath not found: "+absCSV)
	}

	outPath, err := validator.ValidateOutputPath(p.OutputPath, m.baseDir)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}

	rows, err := m.db.CompareExternal(p.PackageName, p.TableName, absCSV, p.TopN)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}

	ext := ".csv"
	if p.OutputFormat == "html" {
		ext = ".html"
	}
	if p.OutputPath == "" {
		outPath = filepath.Join(m.baseDir, fmt.Sprintf("external_%s_%s%s",
			p.PackageName, p.TableName, ext))
	}

	if p.OutputFormat == "html" {
		err = report.WriteExternalHTML(rows, p.TableName, outPath)
	} else {
		err = report.WriteExternalCSV(rows, outPath)
	}
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}

	logger.Infof("compareExternal: %d rows → %s", len(rows), outPath)
	return NewResultResponse(req.ID, map[string]interface{}{
		"rows":       len(rows),
		"outputPath": outPath,
		"format":     p.OutputFormat,
	})
}

// ExportTable 处理 flash.exportTable 请求，将指定批次数据导出为 CSV（按 Size 降序）。
//
// 输入（JSON 字段）:
//   - packageName: 包名（必填）
//   - tableName:   批次标签（必填）
//   - outputPath:  输出文件路径（可选）
//
// 输出: { rows, outputPath }
type exportTableParams struct {
	PackageName string `json:"packageName"`
	TableName   string `json:"tableName"`
	OutputPath  string `json:"outputPath,omitempty"`
}

func (m *Methods) ExportTable(req Request) Response {
	var p exportTableParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}
	if err := validateNames(p.PackageName, p.TableName); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}

	outPath, err := validator.ValidateOutputPath(p.OutputPath, m.baseDir)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}
	if p.OutputPath == "" {
		outPath = filepath.Join(m.baseDir, fmt.Sprintf("export_%s_%s.csv", p.PackageName, p.TableName))
	}

	rows, err := m.db.ExportTable(p.PackageName, p.TableName)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}

	if err := report.WriteExportCSV(rows, outPath); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}

	logger.Infof("exportTable: package=%q table=%q rows=%d → %s", p.PackageName, p.TableName, len(rows), outPath)
	return NewResultResponse(req.ID, map[string]interface{}{
		"rows":       len(rows),
		"outputPath": outPath,
	})
}

// DeleteTable 处理 flash.deleteTable 请求，删除指定批次的全部数据行。
//
// 输入（JSON 字段）:
//   - packageName: 包名（必填）
//   - tableName:   要删除的批次标签（必填）
//
// 输出: { rowsDeleted }
type deleteTableParams struct {
	PackageName string `json:"packageName"`
	TableName   string `json:"tableName"`
}

func (m *Methods) DeleteTable(req Request) Response {
	var p deleteTableParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}
	if err := validateNames(p.PackageName, p.TableName); err != nil {
		return NewErrorResponse(req.ID, ErrCodeInvalidParams, err.Error())
	}

	rowsDeleted, err := m.db.DeleteTable(p.PackageName, p.TableName)
	if err != nil {
		return NewErrorResponse(req.ID, ErrCodeInternal, err.Error())
	}

	logger.Infof("deleteTable: package=%q table=%q rowsDeleted=%d", p.PackageName, p.TableName, rowsDeleted)
	return NewResultResponse(req.ID, map[string]interface{}{
		"rowsDeleted": rowsDeleted,
	})
}

// GetLogs 处理 flash.getLogs 请求，返回内存环形缓冲区中最近的日志条目。
//
// 输入（JSON 字段）:
//   - last: 返回条数（可选，0 或省略=全部）
//
// 输出: { entries: [...], count }
type getLogsParams struct {
	Last int `json:"last,omitempty"`
}

func (m *Methods) GetLogs(req Request) Response {
	var p getLogsParams
	if req.Params != nil {
		_ = json.Unmarshal(req.Params, &p)
	}

	l := logger.Global()
	if l == nil {
		return NewResultResponse(req.ID, map[string]interface{}{"entries": []interface{}{}})
	}

	entries := l.GetLast(p.Last)
	return NewResultResponse(req.ID, map[string]interface{}{
		"entries": entries,
		"count":   len(entries),
	})
}

func validateNames(names ...string) error {
	for _, n := range names {
		if err := validator.ValidateName(n); err != nil {
			return err
		}
	}
	return nil
}

func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

func mustMarshalString(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
