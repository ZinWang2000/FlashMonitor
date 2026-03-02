package test

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"flashmonitor/internal/db"
	"flashmonitor/internal/report"
	"flashmonitor/internal/rpc"
)

func makeTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func makeTestCSV(t *testing.T, rows [][]string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test_*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := csv.NewWriter(f)

	_ = w.Write([]string{"Section", "ModuleName", "FileName", "Size", "MangledName"})
	for _, row := range rows {
		_ = w.Write(row)
	}
	w.Flush()
	return f.Name()
}

func TestImportAndList(t *testing.T) {
	d := makeTestDB(t)

	csvPath := makeTestCSV(t, [][]string{
		{".text", "core", "main.o", "1024", "_Zmain"},
		{".text", "hal", "hal.o", "512", "_Zhal"},
		{".data", "core", "data.o", "256", "_Zdata"},
	})

	result, err := d.ImportCSV("mypkg", "v1.0", csvPath)
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if result.RowsInserted != 3 {
		t.Errorf("expected 3 rows, got %d", result.RowsInserted)
	}

	tables, err := d.ListTables("mypkg")
	if err != nil {
		t.Fatalf("ListTables: %v", err)
	}
	if len(tables) != 1 || tables[0] != "v1.0" {
		t.Errorf("expected [v1.0], got %v", tables)
	}
}

func TestCompareAll(t *testing.T) {
	d := makeTestDB(t)

	csv1 := makeTestCSV(t, [][]string{
		{".text", "core", "main.o", "1000", ""},
		{".text", "hal", "hal.o", "500", ""},
	})
	csv2 := makeTestCSV(t, [][]string{
		{".text", "core", "main.o", "1200", ""},
		{".text", "hal", "hal.o", "400", ""},
		{".data", "core", "new.o", "300", ""},
	})

	if _, err := d.ImportCSV("pkg", "v1", csv1); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ImportCSV("pkg", "v2", csv2); err != nil {
		t.Fatal(err)
	}

	rows, err := d.CompareAll("pkg", "v1", "v2")
	if err != nil {
		t.Fatalf("CompareAll: %v", err)
	}

	found := map[string]int64{}
	for _, r := range rows {
		found[r.FileName] = r.DSize
	}
	if found["main.o"] != 200 {
		t.Errorf("main.o dSize: expected 200, got %d", found["main.o"])
	}
	if found["hal.o"] != -100 {
		t.Errorf("hal.o dSize: expected -100, got %d", found["hal.o"])
	}
	if found["new.o"] != 300 {
		t.Errorf("new.o dSize: expected 300, got %d", found["new.o"])
	}
}

func TestCompareTopN(t *testing.T) {
	d := makeTestDB(t)

	csv1 := makeTestCSV(t, [][]string{
		{".text", "m", "a.o", "100", ""},
		{".text", "m", "b.o", "200", ""},
		{".text", "m", "c.o", "300", ""},
	})
	csv2 := makeTestCSV(t, [][]string{
		{".text", "m", "a.o", "500", ""},
		{".text", "m", "b.o", "150", ""},
		{".text", "m", "c.o", "350", ""},
	})

	d.ImportCSV("p", "t1", csv1)
	d.ImportCSV("p", "t2", csv2)

	rows, err := d.CompareTopN("p", "t1", "t2", 2)
	if err != nil {
		t.Fatalf("CompareTopN: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].FileName != "a.o" || rows[0].DSize != 400 {
		t.Errorf("row 0: expected a.o dSize=400, got %s/%d", rows[0].FileName, rows[0].DSize)
	}
}

func TestTimeSeries(t *testing.T) {
	d := makeTestDB(t)

	csvs := []struct {
		table string
		rows  [][]string
	}{
		{"t1", [][]string{{".text", "m", "a.o", "100", ""}, {".text", "m", "b.o", "50", ""}}},
		{"t2", [][]string{{".text", "m", "a.o", "150", ""}, {".text", "m", "b.o", "60", ""}}},
		{"t3", [][]string{{".text", "m", "a.o", "200", ""}, {".text", "m", "b.o", "40", ""}}},
	}

	for _, c := range csvs {
		p := makeTestCSV(t, c.rows)
		d.ImportCSV("ts_pkg", c.table, p)
	}

	tsRows, err := d.TimeSeries("ts_pkg", "", []string{"t1", "t2", "t3"})
	if err != nil {
		t.Fatalf("TimeSeries: %v", err)
	}
	if len(tsRows) == 0 {
		t.Fatal("expected time series rows")
	}

	for _, r := range tsRows {
		if r.FileName == "a.o" {
			if len(r.Sizes) != 3 {
				t.Errorf("a.o: expected 3 sizes, got %d", len(r.Sizes))
			}
			if r.DSize != 100 {
				t.Errorf("a.o dSize: expected 100, got %d", r.DSize)
			}
		}
	}
}

func TestCompareExternal(t *testing.T) {
	d := makeTestDB(t)

	dbCSV := makeTestCSV(t, [][]string{
		{".text", "m", "file1.o", "1000", ""},
		{".text", "m", "file2.o", "500", ""},
	})
	extCSV := makeTestCSV(t, [][]string{
		{".text", "m", "file1.o", "1100", ""},
		{".text", "m", "file3.o", "800", ""},
	})

	d.ImportCSV("extpkg", "baseline", dbCSV)

	rows, err := d.CompareExternal("extpkg", "baseline", extCSV, 0)
	if err != nil {
		t.Fatalf("CompareExternal: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected rows from external comparison")
	}

	found := map[string]int64{}
	for _, r := range rows {
		found[r.FileName] = r.DSize
	}
	if found["file1.o"] != 100 {
		t.Errorf("file1.o dSize: expected 100, got %d", found["file1.o"])
	}
}

func TestWriteCompareCSV(t *testing.T) {
	rows := []db.CompareRow{
		{Section: ".text", ModuleName: "m", FileName: "a.o", Size1: 100, Size2: 200, DSize: 100},
		{Section: ".data", ModuleName: "m", FileName: "b.o", Size1: 50, Size2: 30, DSize: -20},
	}

	outPath := filepath.Join(t.TempDir(), "out.csv")
	if err := report.WriteCompareCSV(rows, outPath); err != nil {
		t.Fatal(err)
	}

	f, _ := os.Open(outPath)
	defer f.Close()
	records, _ := csv.NewReader(f).ReadAll()

	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}
	if records[0][0] != "Section" || records[0][6] != "dSize" {
		t.Errorf("bad header: %v", records[0])
	}
	dsize, _ := strconv.ParseInt(records[1][6], 10, 64)
	if dsize != 100 {
		t.Errorf("row 0 dSize: expected 100, got %d", dsize)
	}
}

func TestRPCImport(t *testing.T) {
	d := makeTestDB(t)
	methods := rpc.NewMethods(d, t.TempDir())
	handler := rpc.NewHandler(methods)

	csvPath := makeTestCSV(t, [][]string{
		{".text", "core", "main.o", "1024", "_Zmain"},
	})

	reqJSON, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "flash.import",
		"params":  map[string]interface{}{"csvPath": csvPath, "packageName": "testpkg", "tableName": "batch1"},
		"id":      1,
	})

	resp := handler.Handle(reqJSON)
	if resp.Error != nil {
		t.Fatalf("rpc error: %v", resp.Error)
	}
}

func TestRPCMethodNotFound(t *testing.T) {
	d := makeTestDB(t)
	methods := rpc.NewMethods(d, t.TempDir())
	handler := rpc.NewHandler(methods)

	req, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "flash.nonexistent",
		"id":      1,
	})
	resp := handler.Handle(req)
	if resp.Error == nil || resp.Error.Code != rpc.ErrCodeMethodNotFound {
		t.Errorf("expected method not found error, got %v", resp.Error)
	}
}

func TestRPCInvalidParams(t *testing.T) {
	d := makeTestDB(t)
	methods := rpc.NewMethods(d, t.TempDir())
	handler := rpc.NewHandler(methods)

	req, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "flash.compareAll",
		"params": map[string]interface{}{
			"packageName": "pkg",
			"tableName1":  "t1",
			"tableName2":  "t2",
			"outputPath":  "../../etc/passwd",
		},
		"id": 1,
	})
	resp := handler.Handle(req)
	if resp.Error == nil {
		t.Error("expected error for path traversal, got nil")
	}
}

func TestMCPInitialize(t *testing.T) {
	d := makeTestDB(t)
	methods := rpc.NewMethods(d, t.TempDir())
	handler := rpc.NewHandler(methods)

	req, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"id":      1,
	})
	resp := handler.Handle(req)
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}
}
