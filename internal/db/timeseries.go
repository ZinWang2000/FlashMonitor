package db

import (
	"database/sql"
	"fmt"
	"strings"
)

type TimeSeriesRow struct {
	Section     string
	ModuleName  string
	FileName    string
	MangledName string
	Sizes       []int64
	DSize       int64
}

// TimeSeries 对多个批次执行时序分析，返回每个文件在各批次中的大小及总体变化量。
//
// 输入:
//   - packageName: 包名
//   - filePattern: 正则表达式，过滤 FileName；空字符串表示不过滤
//   - tableNames:  有序批次名列表（至少 1 个，需已通过白名单校验）
//
// 输出:
//   - []TimeSeriesRow: 每行含 FileName 及各批次 Size；dSize = 最后批次 - 第一批次
//   - error: 参数非法或查询失败的原因
//
// 注意事项:
//   - 若某批次在数据库中不存在，该列全部填 0（不报错）
//   - 使用 DuckDB FILTER 聚合语法，参数绑定数量随批次数动态增长
func (d *DB) TimeSeries(packageName, filePattern string, tableNames []string) ([]TimeSeriesRow, error) {
	if len(tableNames) == 0 {
		return nil, fmt.Errorf("TimeSeries: at least one table name is required")
	}

	tbl := TableName(packageName)

	existing, err := d.ListTables(packageName)
	if err != nil {
		return nil, err
	}
	existingSet := make(map[string]bool, len(existing))
	for _, t := range existing {
		existingSet[t] = true
	}

	var selectCols []string
	var args []interface{}
	for i, tn := range tableNames {
		if !existingSet[tn] {
			selectCols = append(selectCols, fmt.Sprintf("CAST(0 AS BIGINT) AS col_%d", i))
		} else {
			col := fmt.Sprintf(
				"COALESCE(MAX(Size) FILTER (WHERE TableName = ?), 0) AS col_%d", i)
			selectCols = append(selectCols, col)
			args = append(args, tn)
		}
	}

	lastIdx := len(tableNames) - 1
	dSizeExpr := fmt.Sprintf("col_%d - col_0", lastIdx)

	var whereParts []string
	inPlaceholders := make([]string, 0, len(tableNames))
	for _, tn := range tableNames {
		if existingSet[tn] {
			inPlaceholders = append(inPlaceholders, "?")
			args = append(args, tn)
		}
	}
	if len(inPlaceholders) > 0 {
		whereParts = append(whereParts, fmt.Sprintf("TableName IN (%s)", strings.Join(inPlaceholders, ", ")))
	}
	if filePattern != "" {
		whereParts = append(whereParts, "regexp_matches(FileName, ?)")
		args = append(args, filePattern)
	}
	whereClause := ""
	if len(whereParts) > 0 {
		whereClause = "WHERE " + strings.Join(whereParts, " AND ")
	}

	colList := strings.Join(selectCols, ",\n\t\t\t")
	query := fmt.Sprintf(`
		SELECT
			Section, ModuleName, FileName, MangledName,
			%s,
			(%s) AS dSize
		FROM (
			SELECT
				COALESCE(ANY_VALUE(Section), '')     AS Section,
				COALESCE(ANY_VALUE(ModuleName), '')  AS ModuleName,
				FileName,
				COALESCE(ANY_VALUE(MangledName), '') AS MangledName,
				%s
			FROM "%s"
			%s
			GROUP BY FileName
		) sub
		ORDER BY dSize DESC`,
		colsForOuter(len(tableNames)), dSizeExpr,
		colList, tbl, whereClause)

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("TimeSeries query: %w", err)
	}
	defer rows.Close()

	return scanTimeSeriesRows(rows, len(tableNames))
}

func colsForOuter(n int) string {
	cols := make([]string, n)
	for i := 0; i < n; i++ {
		cols[i] = fmt.Sprintf("col_%d", i)
	}
	return strings.Join(cols, ", ")
}

func scanTimeSeriesRows(rows *sql.Rows, numCols int) ([]TimeSeriesRow, error) {
	var result []TimeSeriesRow
	scanDest := make([]interface{}, 4+numCols+1)
	for rows.Next() {
		var r TimeSeriesRow
		r.Sizes = make([]int64, numCols)
		scanDest[0] = &r.Section
		scanDest[1] = &r.ModuleName
		scanDest[2] = &r.FileName
		scanDest[3] = &r.MangledName
		for i := 0; i < numCols; i++ {
			scanDest[4+i] = &r.Sizes[i]
		}
		scanDest[4+numCols] = &r.DSize
		if err := rows.Scan(scanDest...); err != nil {
			return nil, fmt.Errorf("scanTimeSeriesRows: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scanTimeSeriesRows rows: %w", err)
	}
	return result, nil
}
