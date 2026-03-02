package db

import (
	"fmt"
)

type CompareRow struct {
	Section     string
	ModuleName  string
	FileName    string
	MangledName string
	Size1       int64
	Size2       int64
	DSize       int64
}

// CompareAll 对比同一包内两个批次的所有文件大小差异，按 dSize 降序返回全量结果。
//
// 输入:
//   - packageName: 包名
//   - tableName1:  基线批次名（旧版本，对应 Size1）
//   - tableName2:  对比批次名（新版本，对应 Size2）
//
// 输出:
//   - []CompareRow: 每行含 Section、ModuleName、FileName、MangledName、Size1、Size2、dSize
//   - error: 查询失败的原因
func (d *DB) CompareAll(packageName, tableName1, tableName2 string) ([]CompareRow, error) {
	return d.compareQuery(packageName, tableName1, tableName2, 0)
}

// CompareTopN 对比两个批次，返回 dSize 绝对值最大的前 N 条记录。
//
// 输入:
//   - topN: 返回行数上限；≤0 时返回全部行
//
// 其余输入、输出和错误含义同 CompareAll。
func (d *DB) CompareTopN(packageName, tableName1, tableName2 string, topN int) ([]CompareRow, error) {
	return d.compareQuery(packageName, tableName1, tableName2, topN)
}

// compareQuery 执行批次对比查询的内部实现。
// 使用 FULL OUTER JOIN 保证两侧独有文件均会出现在结果中（缺失侧 Size 以 0 填充）。
//
// 输入:
//   - t1, t2: 两个批次名
//   - topN:   > 0 时追加 LIMIT 子句；否则返回全部行
//
// 注意事项:
//   - 子查询按 FileName GROUP BY 后取 MAX，确保每个文件恰好一行，避免笛卡尔积
func (d *DB) compareQuery(packageName, t1, t2 string, topN int) ([]CompareRow, error) {
	tbl := TableName(packageName)

	limitClause := ""
	args := []interface{}{t1, t2}
	if topN > 0 {
		limitClause = "LIMIT ?"
		args = append(args, topN)
	}

	query := fmt.Sprintf(`
		SELECT
			COALESCE(a.Section,     b.Section,     '') AS Section,
			COALESCE(a.ModuleName,  b.ModuleName,  '') AS ModuleName,
			COALESCE(a.FileName,    b.FileName)        AS FileName,
			COALESCE(a.MangledName, b.MangledName, '') AS MangledName,
			COALESCE(a.Size, 0)                        AS Size1,
			COALESCE(b.Size, 0)                        AS Size2,
			COALESCE(b.Size, 0) - COALESCE(a.Size, 0) AS dSize
		FROM (
			SELECT MAX(Section) AS Section, MAX(ModuleName) AS ModuleName,
			       FileName, MAX(MangledName) AS MangledName, MAX(Size) AS Size
			FROM "%s"
			WHERE TableName = ?
			GROUP BY FileName
		) a
		FULL OUTER JOIN (
			SELECT MAX(Section) AS Section, MAX(ModuleName) AS ModuleName,
			       FileName, MAX(MangledName) AS MangledName, MAX(Size) AS Size
			FROM "%s"
			WHERE TableName = ?
			GROUP BY FileName
		) b ON a.FileName = b.FileName
		ORDER BY dSize DESC
		%s`, tbl, tbl, limitClause)

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("compareQuery(%q, %q, %q): %w", packageName, t1, t2, err)
	}
	defer rows.Close()

	return scanCompareRows(rows)
}

func scanCompareRows(rows interface {
	Next() bool
	Scan(dest ...interface{}) error
	Err() error
}) ([]CompareRow, error) {
	var result []CompareRow
	for rows.Next() {
		var r CompareRow
		if err := rows.Scan(
			&r.Section, &r.ModuleName, &r.FileName,
			&r.MangledName, &r.Size1, &r.Size2, &r.DSize,
		); err != nil {
			return nil, fmt.Errorf("scanCompareRows scan: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scanCompareRows rows: %w", err)
	}
	return result, nil
}
