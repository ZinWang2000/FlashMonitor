package db

import (
	"fmt"
	"path/filepath"
)

type ImportResult struct {
	PackageName  string
	TableName    string
	RowsInserted int64
}

// ImportCSV 将 CSV 文件中的记录批量导入到指定包的数据表中。
//
// 输入:
//   - packageName: 目标包名（需已通过白名单校验）
//   - tableName:   批次标签，作为每条记录的 TableName 字段值
//   - csvPath:     CSV 文件的绝对路径（含标题行）
//
// 输出:
//   - *ImportResult: 包含包名、批次名、写入行数
//   - error: 建表或 SQL 执行失败的原因
//
// 注意事项:
//   - 过滤条件：FileName 非空、Size 可转为非负整数，否则跳过该行
//   - CSV 路径以单引号转义后直接嵌入 SQL，不使用参数绑定（DuckDB 限制）
func (d *DB) ImportCSV(packageName, tableName, csvPath string) (*ImportResult, error) {
	if err := d.EnsurePackageTable(packageName); err != nil {
		return nil, err
	}

	tbl := TableName(packageName)

	var existCount int64
	if err := d.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM "%s" WHERE TableName = ?`, tbl), tableName,
	).Scan(&existCount); err != nil {
		return nil, fmt.Errorf("ImportCSV check existing(%q, %q): %w", packageName, tableName, err)
	}
	if existCount > 0 {
		return nil, fmt.Errorf("批次 %q 在包 %q 中已存在（%d 行）；请先删除该批次再重新导入", tableName, packageName, existCount)
	}

	absPath := escapeSQLPath(filepath.ToSlash(csvPath))

	query := fmt.Sprintf(`
		INSERT INTO "%s" (TableName, Section, ModuleName, FileName, Size, MangledName)
		SELECT
			? AS TableName,
			"Section",
			"ModuleName",
			"FileName",
			TRY_CAST("Size" AS BIGINT),
			"MangledName"
		FROM read_csv_auto('%s', header=true)
		WHERE "FileName" IS NOT NULL AND "FileName" != ''
		  AND TRY_CAST("Size" AS BIGINT) IS NOT NULL
		  AND TRY_CAST("Size" AS BIGINT) >= 0`,
		tbl, absPath)

	res, err := d.Exec(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("ImportCSV(%q, %q): %w", packageName, tableName, err)
	}

	rows, _ := res.RowsAffected()
	return &ImportResult{
		PackageName:  packageName,
		TableName:    tableName,
		RowsInserted: rows,
	}, nil
}
