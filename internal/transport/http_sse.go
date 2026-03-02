package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"flashmonitor/internal/db"
	"flashmonitor/internal/logger"
	"flashmonitor/internal/rpc"
	"flashmonitor/web"
)

type HTTPTransport struct {
	handler *rpc.Handler
	db      *db.DB
	baseDir string
	addr    string
	srv     *http.Server

	mu          sync.Mutex
	subscribers map[chan string]struct{}
}

// NewHTTPTransport 创建 HTTP 传输层实例，绑定 RPC 处理器、数据库连接、输出目录和监听地址。
//
// 输入:
//   - handler:  JSON-RPC 请求分发器
//   - database: DuckDB 连接（供上传接口使用）
//   - baseDir:  报告文件输出根目录
//   - addr:     HTTP 监听地址（如 ":8080"）
func NewHTTPTransport(handler *rpc.Handler, database *db.DB, baseDir, addr string) *HTTPTransport {
	return &HTTPTransport{
		handler:     handler,
		db:          database,
		baseDir:     baseDir,
		addr:        addr,
		subscribers: make(map[chan string]struct{}),
	}
}

// Start 启动 HTTP 服务器，注册所有路由并开始监听请求。
// 路由：POST /rpc（JSON-RPC）、POST /upload（CSV 上传）、GET /events（SSE）、GET /download/{file}、GET /（静态前端）。
//
// 输出:
//   - error: 监听失败原因（http.ErrServerClosed 不被视为错误）
//
// 注意事项:
//   - 此方法阻塞直到服务器关闭，建议在独立 goroutine 中调用
func (t *HTTPTransport) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", t.handleRPC)
	mux.HandleFunc("/upload", t.handleUpload)
	mux.HandleFunc("/events", t.handleSSE)
	mux.HandleFunc("/download/", t.handleDownload)

	mux.Handle("/", http.FileServer(http.FS(web.FS())))

	t.srv = &http.Server{
		Addr:         t.addr,
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 300 * time.Second,
	}

	logger.Infof("HTTP server starting on %s", t.addr)
	if err := t.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// Stop 关闭 HTTP 服务器，等待进行中的请求完成（最多 5 秒）。
//
// 输出:
//   - error: 关闭失败原因
func (t *HTTPTransport) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return t.srv.Shutdown(ctx)
}

func (t *HTTPTransport) handleRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	resp := t.handler.Handle(body)
	out, _ := json.Marshal(resp)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, _ = w.Write(out)

	t.broadcast(string(out))
}

func (t *HTTPTransport) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := r.ParseMultipartForm(256 * 1024 * 1024); err != nil {
		http.Error(w, "parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "no file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	packageName := r.FormValue("packageName")
	tableName := r.FormValue("tableName")

	if packageName == "" {
		http.Error(w, "packageName is required", http.StatusBadRequest)
		return
	}

	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, "flashmonitor_upload_"+fmt.Sprintf("%d", time.Now().UnixNano())+"_"+filepath.Base(header.Filename))
	f, err := os.Create(tmpFile)
	if err != nil {
		http.Error(w, "create temp file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(f, file); err != nil {
		f.Close()
		os.Remove(tmpFile)
		http.Error(w, "write temp file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	f.Close()
	defer os.Remove(tmpFile)

	params := map[string]interface{}{
		"csvPath":     tmpFile,
		"packageName": packageName,
	}
	if tableName != "" {
		params["tableName"] = tableName
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "flash.import",
		"params":  params,
		"id":      1,
	})

	resp := t.handler.Handle(reqBody)
	out, _ := json.Marshal(resp)

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func (t *HTTPTransport) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan string, 100)
	t.mu.Lock()
	t.subscribers[ch] = struct{}{}
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		delete(t.subscribers, ch)
		t.mu.Unlock()
	}()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	_, _ = fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			_, _ = fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (t *HTTPTransport) handleDownload(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/download/")
	if name == "" || strings.Contains(name, "..") {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	filePath := filepath.Join(t.baseDir, name)
	cleanBase := filepath.Clean(t.baseDir) + string(filepath.Separator)
	cleanFile := filepath.Clean(filePath) + string(filepath.Separator)
	if !strings.HasPrefix(cleanFile, cleanBase) {
		http.Error(w, "path traversal", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, filePath)
}

func (t *HTTPTransport) broadcast(msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for ch := range t.subscribers {
		select {
		case ch <- msg:
		default:
		}
	}
}
