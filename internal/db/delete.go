package db

import "fmt"

// DeleteTable 删除指定包内某批次的全部数据行。
//
// 输入:
//   - packageName: 包名
//   - tableName:   要删除的批次标签
//
// 输出:
//   - int64: 实际删除的行数（批次不存在时为 0）
//   - error: 包不存在或 SQL 执行失败的原因
//
// 注意事项:
//   - 仅删除行数据，不删除物理表结构
//   - 若包对应的表不存在，返回错误而非静默成功
func (d *DB) DeleteTable(packageName, tableName string) (int64, error) {

	pkgs, err := d.ListPackages()
	if err != nil {
		return 0, err
	}
	found := false
	for _, p := range pkgs {
		if p == packageName {
			found = true
			break
		}
	}
	if !found {
		return 0, fmt.Errorf("包 %q 不存在", packageName)
	}

	tbl := TableName(packageName)
	res, err := d.Exec(
		fmt.Sprintf(`DELETE FROM "%s" WHERE TableName = ?`, tbl), tableName,
	)
	if err != nil {
		return 0, fmt.Errorf("DeleteTable(%q, %q): %w", packageName, tableName, err)
	}
	rows, _ := res.RowsAffected()
	return rows, nil
}
