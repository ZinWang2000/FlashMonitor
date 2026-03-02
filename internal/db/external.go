package db

import (
	"fmt"
	"path/filepath"
)

type ExternalCompareRow struct {
	Section     string
	ModuleName  string
	FileName    string
	MangledName string
	Size1       int64
	Size2       int64
	DSize       int64
}

// CompareExternal 将数据库中一个批次与本地外部 CSV 文件做全外连接对比。
//
// 输入:
//   - packageName:     包名
//   - tableName:       数据库内的批次名（对应 Size1）
//   - externalCsvPath: 外部 CSV 文件的绝对路径（对应 Size2）
//   - topN:            ≤0 返回全部行；>0 追加 LIMIT
//
// 输出:
//   - []ExternalCompareRow: dSize = Size2 - Size1（正值表示外部 CSV 更大）
//   - error: 查询失败的原因
//
// 注意事项:
//   - 外部 CSV 路径以单引号转义后嵌入 SQL（DuckDB read_csv_auto 限制）
//   - 外部 CSV 的 Size 字段若非数字则以 0 替代（TRY_CAST + COALESCE）
func (d *DB) CompareExternal(packageName, tableName, externalCsvPath string, topN int) ([]ExternalCompareRow, error) {
	tbl := TableName(packageName)
	absPath := escapeSQLPath(filepath.ToSlash(externalCsvPath))

	limitClause := ""
	args := []interface{}{tableName}
	if topN > 0 {
		limitClause = "LIMIT ?"
	}

	query := fmt.Sprintf(`
		SELECT
			COALESCE(db.Section,     ext.Section,     '') AS Section,
			COALESCE(db.ModuleName,  ext.ModuleName,  '') AS ModuleName,
			COALESCE(db.FileName,    ext.FileName,    '') AS FileName,
			COALESCE(db.MangledName, ext.MangledName, '') AS MangledName,
			COALESCE(db.Size, 0)                      AS Size1,
			COALESCE(ext.Size, 0)                     AS Size2,
			COALESCE(ext.Size, 0) - COALESCE(db.Size, 0) AS dSize
		FROM
			(SELECT Section, ModuleName, FileName, MangledName, Size
			 FROM "%s"
			 WHERE TableName = ?) AS db
		FULL OUTER JOIN
			(SELECT
				"Section",
				"ModuleName",
				"FileName",
				"MangledName",
				COALESCE(TRY_CAST("Size" AS BIGINT), 0) AS Size
			 FROM read_csv_auto('%s', header=true)
			 WHERE "FileName" IS NOT NULL AND "FileName" != '') AS ext
		ON db.FileName = ext.FileName
		ORDER BY dSize DESC
		%s`, tbl, absPath, limitClause)

	if topN > 0 {
		args = append(args, topN)
	}

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("CompareExternal(%q, %q): %w", packageName, tableName, err)
	}
	defer rows.Close()

	var result []ExternalCompareRow
	for rows.Next() {
		var r ExternalCompareRow
		if err := rows.Scan(
			&r.Section, &r.ModuleName, &r.FileName,
			&r.MangledName, &r.Size1, &r.Size2, &r.DSize,
		); err != nil {
			return nil, fmt.Errorf("CompareExternal scan: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("CompareExternal rows: %w", err)
	}
	return result, nil
}
