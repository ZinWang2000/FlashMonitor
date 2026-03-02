package report

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"

	"flashmonitor/internal/db"
)

// WriteCompareCSV 将批次对比结果写入 CSV 文件。
//
// 输入:
//   - rows:       对比结果行（CompareRow 切片）
//   - outputPath: 输出文件路径；为空时写入 stdout
//
// 输出:
//   - error: 文件创建或写入失败的原因
//
// 注意事项:
//   - 列顺序：Section, ModuleName, FileName, MangledName, Size1, Size2, dSize
func WriteCompareCSV(rows []db.CompareRow, outputPath string) error {
	w, closer, err := openOutput(outputPath)
	if err != nil {
		return err
	}
	defer closer()

	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"Section", "ModuleName", "FileName", "MangledName", "Size1", "Size2", "dSize",
	}); err != nil {
		return fmt.Errorf("WriteCompareCSV header: %w", err)
	}
	for _, r := range rows {
		if err := cw.Write([]string{
			r.Section, r.ModuleName, r.FileName, r.MangledName,
			strconv.FormatInt(r.Size1, 10),
			strconv.FormatInt(r.Size2, 10),
			strconv.FormatInt(r.DSize, 10),
		}); err != nil {
			return fmt.Errorf("WriteCompareCSV row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteExternalCSV 将外部 CSV 对比结果写入 CSV 文件，列格式同 WriteCompareCSV。
//
// 输入:
//   - rows:       外部对比结果行（ExternalCompareRow 切片）
//   - outputPath: 输出文件路径；为空时写入 stdout
func WriteExternalCSV(rows []db.ExternalCompareRow, outputPath string) error {
	w, closer, err := openOutput(outputPath)
	if err != nil {
		return err
	}
	defer closer()

	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"Section", "ModuleName", "FileName", "MangledName", "Size1", "Size2", "dSize",
	}); err != nil {
		return fmt.Errorf("WriteExternalCSV header: %w", err)
	}
	for _, r := range rows {
		if err := cw.Write([]string{
			r.Section, r.ModuleName, r.FileName, r.MangledName,
			strconv.FormatInt(r.Size1, 10),
			strconv.FormatInt(r.Size2, 10),
			strconv.FormatInt(r.DSize, 10),
		}); err != nil {
			return fmt.Errorf("WriteExternalCSV row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteTimeSeriesCSV 将时序分析结果写入 CSV 文件。
//
// 输入:
//   - rows:       时序分析结果行（每行含各批次 Size）
//   - tableNames: 有序批次名列表，用作动态列标题
//   - outputPath: 输出文件路径；为空时写入 stdout
//
// 注意事项:
//   - 列顺序：Section, ModuleName, FileName, MangledName, <批次名...>, dSize
func WriteTimeSeriesCSV(rows []db.TimeSeriesRow, tableNames []string, outputPath string) error {
	w, closer, err := openOutput(outputPath)
	if err != nil {
		return err
	}
	defer closer()

	cw := csv.NewWriter(w)
	header := append([]string{"Section", "ModuleName", "FileName", "MangledName"}, tableNames...)
	header = append(header, "dSize")
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("WriteTimeSeriesCSV header: %w", err)
	}
	for _, r := range rows {
		record := []string{r.Section, r.ModuleName, r.FileName, r.MangledName}
		for _, s := range r.Sizes {
			record = append(record, strconv.FormatInt(s, 10))
		}
		record = append(record, strconv.FormatInt(r.DSize, 10))
		if err := cw.Write(record); err != nil {
			return fmt.Errorf("WriteTimeSeriesCSV row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteExportCSV 将单批次导出结果写入 CSV 文件（按 Size 降序，由调用方保证）。
//
// 输入:
//   - rows:       导出结果行（ExportRow 切片）
//   - outputPath: 输出文件路径；为空时写入 stdout
//
// 注意事项:
//   - 列顺序：Section, ModuleName, FileName, MangledName, Size
func WriteExportCSV(rows []db.ExportRow, outputPath string) error {
	w, closer, err := openOutput(outputPath)
	if err != nil {
		return err
	}
	defer closer()

	cw := csv.NewWriter(w)
	if err := cw.Write([]string{
		"Section", "ModuleName", "FileName", "MangledName", "Size",
	}); err != nil {
		return fmt.Errorf("WriteExportCSV header: %w", err)
	}
	for _, r := range rows {
		if err := cw.Write([]string{
			r.Section, r.ModuleName, r.FileName, r.MangledName,
			strconv.FormatInt(r.Size, 10),
		}); err != nil {
			return fmt.Errorf("WriteExportCSV row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

func openOutput(path string) (io.Writer, func(), error) {
	if path == "" {
		return os.Stdout, func() {}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, fmt.Errorf("openOutput create %q: %w", path, err)
	}
	return f, func() { _ = f.Close() }, nil
}
