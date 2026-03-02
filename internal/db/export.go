package db

import "fmt"

// ExportRow 表示单批次导出的一条记录。
type ExportRow struct {
	Section     string
	ModuleName  string
	FileName    string
	MangledName string
	Size        int64
}

// ExportTable 导出指定包内某批次的所有文件记录，按 Size 降序排列。
//
// 输入:
//   - packageName: 包名
//   - tableName:   批次标签
//
// 输出:
//   - []ExportRow: 按 Size 降序排列的记录列表
//   - error: 查询失败的原因
//
// 注意事项:
//   - 同一 FileName 若有多行（理论上不应出现），取 MAX(Size)
func (d *DB) ExportTable(packageName, tableName string) ([]ExportRow, error) {
	if err := d.EnsurePackageTable(packageName); err != nil {
		return nil, err
	}
	tbl := TableName(packageName)

	query := fmt.Sprintf(`
		SELECT
			COALESCE(MAX(Section), '')     AS Section,
			COALESCE(MAX(ModuleName), '')  AS ModuleName,
			FileName,
			COALESCE(MAX(MangledName), '') AS MangledName,
			MAX(Size)                      AS Size
		FROM "%s"
		WHERE TableName = ?
		GROUP BY FileName
		ORDER BY Size DESC`, tbl)

	rows, err := d.Query(query, tableName)
	if err != nil {
		return nil, fmt.Errorf("ExportTable(%q, %q): %w", packageName, tableName, err)
	}
	defer rows.Close()

	var result []ExportRow
	for rows.Next() {
		var r ExportRow
		if err := rows.Scan(&r.Section, &r.ModuleName, &r.FileName, &r.MangledName, &r.Size); err != nil {
			return nil, fmt.Errorf("ExportTable scan: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ExportTable rows: %w", err)
	}
	return result, nil
}
