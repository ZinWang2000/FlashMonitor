package db

import (
	"fmt"
	"sort"
	"strings"
)

type PackageInfo struct {
	Name   string   `json:"name"`
	Tables []string `json:"tables"`
}

// ListTables 返回指定包内所有批次名称（TableName 去重），按字母顺序排列。
//
// 输入:
//   - packageName: 包名
//
// 输出:
//   - []string: 批次名列表；包不存在时自动建表并返回空切片
//   - error: 建表或查询失败的原因
func (d *DB) ListTables(packageName string) ([]string, error) {
	if err := d.EnsurePackageTable(packageName); err != nil {
		return nil, err
	}
	tbl := TableName(packageName)

	rows, err := d.Query(fmt.Sprintf(`
		SELECT DISTINCT TableName
		FROM "%s"
		ORDER BY TableName ASC`, tbl))
	if err != nil {
		return nil, fmt.Errorf("ListTables(%q): %w", packageName, err)
	}
	defer rows.Close()

	tables := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("ListTables scan: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListTables rows: %w", err)
	}

	sort.Strings(tables)
	return tables, nil
}

// ListPackages 扫描 information_schema 返回数据库中所有 pkg_* 表对应的包名列表。
//
// 输出:
//   - []string: 包名列表（已去除 "pkg_" 前缀），按字母顺序排列
//   - error: 查询失败的原因
func (d *DB) ListPackages() ([]string, error) {
	rows, err := d.Query(`
		SELECT table_name
		FROM information_schema.tables
		WHERE table_type = 'BASE TABLE'
		  AND table_name LIKE 'pkg_%'
		ORDER BY table_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("ListPackages: %w", err)
	}
	defer rows.Close()

	var packages []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("ListPackages scan: %w", err)
		}

		if len(name) > 4 {
			packages = append(packages, name[4:])
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListPackages rows: %w", err)
	}
	return packages, nil
}

// listTablesRaw 直接查询已存在的 pkg 表的批次列表，不执行任何 DDL。
//
// 输入:
//   - packageName: 包名（调用方已确认对应物理表存在）
//
// 输出:
//   - []string: 批次名列表，按字母顺序排列
//   - error: 查询失败的原因
func (d *DB) listTablesRaw(packageName string) ([]string, error) {
	tbl := TableName(packageName)
	rows, err := d.Query(fmt.Sprintf(`
		SELECT DISTINCT TableName
		FROM "%s"
		ORDER BY TableName ASC`, tbl))
	if err != nil {
		return nil, fmt.Errorf("listTablesRaw(%q): %w", packageName, err)
	}
	defer rows.Close()
	tables := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("listTablesRaw scan: %w", err)
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// ListAllPackagesWithTables 返回所有包及其批次名列表。
//
// 输出:
//   - []PackageInfo: 每个元素含包名 (Name) 和批次列表 (Tables)
//   - error: 查询失败的原因
//
// 注意事项:
//   - 使用单次 UNION ALL 查询取代逐包查询，复杂度从 O(N) 次 DB 往返降至 O(1)
//   - 不执行任何 DDL，纯只读操作；空包（无批次）的 Tables 为空切片
func (d *DB) ListAllPackagesWithTables() ([]PackageInfo, error) {
	packages, err := d.ListPackages()
	if err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		return []PackageInfo{}, nil
	}

	var sb strings.Builder
	for i, pkg := range packages {
		if i > 0 {
			sb.WriteString("\nUNION ALL\n")
		}
		tbl := TableName(pkg)

		fmt.Fprintf(&sb, `SELECT DISTINCT '%s' AS pkg, TableName FROM "%s"`,
			strings.ReplaceAll(pkg, "'", "''"), tbl)
	}
	sb.WriteString("\nORDER BY pkg ASC, TableName ASC")

	rows, err := d.Query(sb.String())
	if err != nil {
		return nil, fmt.Errorf("ListAllPackagesWithTables union: %w", err)
	}
	defer rows.Close()

	tableMap := make(map[string][]string, len(packages))
	for _, p := range packages {
		tableMap[p] = []string{}
	}
	for rows.Next() {
		var pkg, tname string
		if err := rows.Scan(&pkg, &tname); err != nil {
			return nil, fmt.Errorf("ListAllPackagesWithTables scan: %w", err)
		}
		tableMap[pkg] = append(tableMap[pkg], tname)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ListAllPackagesWithTables rows: %w", err)
	}

	result := make([]PackageInfo, 0, len(packages))
	for _, pkg := range packages {
		result = append(result, PackageInfo{Name: pkg, Tables: tableMap[pkg]})
	}
	return result, nil
}
