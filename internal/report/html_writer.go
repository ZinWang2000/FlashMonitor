package report

import (
	"fmt"
	"html/template"
	"os"
	"strconv"

	"flashmonitor/internal/db"
)

var echartsScript = "https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js"

func SetEChartsScript(urlOrPath string) {
	echartsScript = urlOrPath
}

const compareHTMLTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>FlashMonitor: {{.Title}}</title>
<script src="{{.EChartsURL}}"></script>
<style>
body { font-family: sans-serif; margin: 20px; background: #f5f5f5; }
h1 { color: #333; }
#chart { width: 100%; height: {{.ChartHeight}}px; background: #fff; border-radius: 8px; padding: 10px; box-sizing: border-box; }
table { border-collapse: collapse; width: 100%; margin-top: 20px; background: #fff; border-radius: 8px; overflow: hidden; }
th, td { padding: 8px 12px; border: 1px solid #ddd; text-align: right; }
th { background: #4a90d9; color: #fff; text-align: center; }
td:nth-child(1) { color: #888; }
td:nth-child(2), td:nth-child(3), td:nth-child(4), td:nth-child(5) { text-align: left; }
.positive { color: #e74c3c; }
.negative { color: #27ae60; }
</style>
</head>
<body>
<h1>{{.Title}}</h1>
<p>Table1: <strong>{{.TableName1}}</strong> → Table2: <strong>{{.TableName2}}</strong></p>
<div id="chart"></div>
<table>
<thead><tr><th>#</th><th>Section</th><th>Module</th><th>FileName</th><th>MangledName</th><th>Size1</th><th>Size2</th><th>dSize</th></tr></thead>
<tbody>
{{range .Rows}}<tr>
<td>{{.Index}}</td>
<td>{{.Section}}</td>
<td>{{.ModuleName}}</td>
<td>{{.FileName}}</td>
<td>{{.MangledName}}</td>
<td>{{.Size1}}</td>
<td>{{.Size2}}</td>
<td class="{{.DSizeClass}}">{{.DSize}}</td>
</tr>{{end}}
</tbody>
</table>
<script>
var chart = echarts.init(document.getElementById('chart'));
var names = [{{range .Rows}}'{{js .FileName}}',{{end}}];
var dsizes = [{{range .Rows}}{{.DSize}},{{end}}];
chart.setOption({
  tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
  grid: { left: '30%', right: '5%', top: '5%', bottom: '5%' },
  xAxis: { type: 'value', name: 'dSize (bytes)' },
  yAxis: {
    type: 'category',
    data: names.slice().reverse(),
    axisLabel: { width: 300, overflow: 'truncate', fontSize: 11 }
  },
  series: [{
    name: 'dSize',
    type: 'bar',
    data: dsizes.slice().reverse(),
    itemStyle: {
      color: function(params) { return params.value >= 0 ? '#e74c3c' : '#27ae60'; }
    },
    label: { show: true, position: 'right', formatter: '{c}' }
  }]
});
window.addEventListener('resize', function() { chart.resize(); });
</script>
</body>
</html>`

type compareHTMLData struct {
	Title       string
	EChartsURL  string
	TableName1  string
	TableName2  string
	ChartHeight int
	Rows        []compareHTMLRow
}

type compareHTMLRow struct {
	Index       int
	Section     string
	ModuleName  string
	FileName    string
	MangledName string
	Size1       string
	Size2       string
	DSize       int64
	DSizeClass  string
}

// WriteCompareHTML 生成包含 ECharts 水平柱状图和详情表格的 HTML 报告。
//
// 输入:
//   - rows:        对比结果行（至少 1 行）
//   - tableName1:  基线批次名（显示在报告标题中）
//   - tableName2:  对比批次名
//   - outputPath:  输出 HTML 文件路径
//
// 输出:
//   - error: 数据为空、模板解析或文件写入失败的原因
//
// 注意事项:
//   - 图表高度自动根据行数计算，最小 400px，最大 2000px
//   - dSize > 0 显示红色（增大），dSize < 0 显示绿色（减小）
func WriteCompareHTML(rows []db.CompareRow, tableName1, tableName2, outputPath string) error {
	if len(rows) == 0 {
		return fmt.Errorf("no data to render")
	}

	tmpl, err := template.New("compare").Funcs(template.FuncMap{
		"js": template.JSEscapeString,
	}).Parse(compareHTMLTmpl)
	if err != nil {
		return fmt.Errorf("WriteCompareHTML parse template: %w", err)
	}

	htmlRows := make([]compareHTMLRow, len(rows))
	for i, r := range rows {
		cls := ""
		if r.DSize > 0 {
			cls = "positive"
		} else if r.DSize < 0 {
			cls = "negative"
		}
		htmlRows[i] = compareHTMLRow{
			Index:       i + 1,
			Section:     r.Section,
			ModuleName:  r.ModuleName,
			FileName:    r.FileName,
			MangledName: r.MangledName,
			Size1:       strconv.FormatInt(r.Size1, 10),
			Size2:       strconv.FormatInt(r.Size2, 10),
			DSize:       r.DSize,
			DSizeClass:  cls,
		}
	}

	chartHeight := len(rows)*20 + 100
	if chartHeight < 400 {
		chartHeight = 400
	}
	if chartHeight > 2000 {
		chartHeight = 2000
	}

	data := compareHTMLData{
		Title:       fmt.Sprintf("Size Comparison: %s vs %s", tableName1, tableName2),
		EChartsURL:  echartsScript,
		TableName1:  tableName1,
		TableName2:  tableName2,
		ChartHeight: chartHeight,
		Rows:        htmlRows,
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("WriteCompareHTML create %q: %w", outputPath, err)
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

// WriteExternalHTML 生成外部 CSV 对比的 HTML 报告（复用 WriteCompareHTML 模板）。
//
// 输入:
//   - rows:       外部对比结果行
//   - tableName:  数据库批次名（作为 Table1 显示）
//   - outputPath: 输出 HTML 文件路径
//
// 注意事项:
//   - Table2 标签固定显示为 "external CSV"
func WriteExternalHTML(rows []db.ExternalCompareRow, tableName, outputPath string) error {

	compareRows := make([]db.CompareRow, len(rows))
	for i, r := range rows {
		compareRows[i] = db.CompareRow{
			Section:     r.Section,
			ModuleName:  r.ModuleName,
			FileName:    r.FileName,
			MangledName: r.MangledName,
			Size1:       r.Size1,
			Size2:       r.Size2,
			DSize:       r.DSize,
		}
	}
	return WriteCompareHTML(compareRows, tableName, "external CSV", outputPath)
}
