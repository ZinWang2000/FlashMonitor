package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/marcboeker/go-duckdb"
)

type DB struct {
	*sql.DB
}

// Open 打开（或创建）指定路径的 DuckDB 数据库，返回包装后的 DB 实例。
//
// 输入:
//   - path: 数据库文件路径；":memory:" 表示内存数据库
//
// 输出:
//   - *DB:  已就绪的数据库连接
//   - error: 打开或 Ping 失败的原因
//
// 注意事项:
//   - MaxOpenConns 固定为 1，确保 DuckDB 嵌入式串行写入不冲突
func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("db.Open: %w", err)
	}

	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("db.Open ping: %w", err)
	}

	return &DB{sqlDB}, nil
}

// TableName 根据包名生成物理表名（格式：pkg_{packageName}）。
//
// 输入:
//   - packageName: 包名标识符（已通过白名单校验）
//
// 输出:
//   - string: 对应的 DuckDB 表名
func TableName(packageName string) string {
	return "pkg_" + packageName
}

// EnsurePackageTable 若包对应的数据表不存在则自动创建，同时创建联合索引。
//
// 输入:
//   - packageName: 包名（用于推导表名和索引名）
//
// 输出:
//   - error: 建表或建索引失败的原因
//
// 注意事项:
//   - 使用 CREATE TABLE IF NOT EXISTS，可安全重复调用
//   - 索引名格式：idx_{packageName}_tf，覆盖 (TableName, FileName) 列
func (d *DB) EnsurePackageTable(packageName string) error {
	tbl := TableName(packageName)
	idxName := "idx_" + packageName + "_tf"

	_, err := d.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s" (
			TableName   VARCHAR NOT NULL,
			Section     VARCHAR,
			ModuleName  VARCHAR,
			FileName    VARCHAR NOT NULL,
			Size        BIGINT  NOT NULL DEFAULT 0,
			MangledName VARCHAR
		)`, tbl))
	if err != nil {
		return fmt.Errorf("EnsurePackageTable create table %q: %w", tbl, err)
	}

	_, err = d.Exec(fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS "%s" ON "%s" (TableName, FileName)`,
		idxName, tbl))
	if err != nil {
		return fmt.Errorf("EnsurePackageTable create index %q: %w", idxName, err)
	}
	return nil
}

func (d *DB) Close() error {
	return d.DB.Close()
}

// escapeSQLPath 将文件路径中的单引号转义，以便安全嵌入 SQL 字符串字面量。
//
// 输入:
//   - p: 已转为正斜杠的文件路径
//
// 输出:
//   - string: 单引号已转义的路径（' → ”）
//
// 注意事项:
//   - DuckDB read_csv_auto 不支持参数化路径，只能嵌入字面量，故需此转义
func escapeSQLPath(p string) string {
	return strings.ReplaceAll(p, "'", "''")
}
