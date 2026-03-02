package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"flashmonitor/internal/db"
	"flashmonitor/internal/logger"
	"flashmonitor/internal/rpc"
	"flashmonitor/internal/transport"
)

var version = "0.1.0-beta"

func main() {
	var (
		stdio   = flag.Bool("stdio", false, "Run in MCP stdio mode (reads JSON-RPC from stdin, writes to stdout)")
		port    = flag.String("port", "8080", "HTTP server port (default: 8080)")
		dbPath  = flag.String("db", "flashmonitor.db", "Path to the DuckDB database file")
		showVer = flag.Bool("version", false, "Show version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("FlashMonitor v%s\n", version)
		os.Exit(0)
	}

	if *stdio {
		flag.Visit(func(f *flag.Flag) {
			if f.Name == "port" {
				log.Fatal("error: --stdio and --port are mutually exclusive")
			}
		})
	}

	absDB, err := filepath.Abs(*dbPath)
	if err != nil {
		log.Fatalf("error resolving db path: %v", err)
	}
	baseDir := filepath.Dir(absDB)

	logDir := filepath.Join(baseDir, "logs")
	if err := logger.Init(logDir); err != nil {
		log.Printf("warning: logger init failed: %v", err)
	}
	defer logger.Global().Close()

	logger.Infof("FlashMonitor v%s starting", version)
	logger.Infof("database: %s", absDB)

	database := openDatabaseWithRetry(absDB, *stdio)
	defer database.Close()

	logger.Info("database opened successfully")

	methods := rpc.NewMethods(database, baseDir)
	handler := rpc.NewHandler(methods)

	if *stdio {
		logger.Info("starting MCP stdio transport")
		t := transport.NewStdioTransport(handler)
		if err := t.Start(); err != nil {
			log.Fatalf("stdio transport error: %v", err)
		}
	} else {
		addr := ":" + *port
		url := fmt.Sprintf("http://localhost:%s", *port)
		logger.Infof("starting HTTP server on %s", addr)
		fmt.Printf("FlashMonitor v%s 已启动，默认访问地址：%s\n", version, url)
		fmt.Println("Have a nice day！—— Author：ZinWang")

		go func() {
			time.Sleep(500 * time.Millisecond)
			if err := exec.Command("cmd", "/c", "start", url).Start(); err != nil {
				logger.Infof("auto open browser failed: %v", err)
			}
		}()

		t := transport.NewHTTPTransport(handler, database, baseDir, addr)
		if err := t.Start(); err != nil {
			log.Fatalf("http transport error: %v", err)
		}
	}
}

const maxDBRetries = 3

// openDatabaseWithRetry 尝试打开数据库，行为如下：
//   - 文件不存在（首次创建）：打印提示后直接连接，失败则 fatal。
//   - 文件已存在但连接失败（可能被占用）：在非 stdio 模式下允许用户按 Enter 重试，
//     最多重试 maxDBRetries 次；超限后打印联系信息并退出。
//   - stdio 模式：任何失败均直接 fatal，避免阻塞标准输入流。
func openDatabaseWithRetry(absDB string, stdioMode bool) *db.DB {
	retries := 0
	for {
		_, statErr := os.Stat(absDB)
		fileExists := statErr == nil

		if !fileExists {
			fmt.Printf("  数据库文件不存在，正在创建：%s\n", absDB)
		}

		database, err := db.Open(absDB)
		if err == nil {
			return database
		}

		if fileExists && !stdioMode {
			retries++
			fmt.Fprintf(os.Stderr, "%v\nERROR:数据库连接失败,按Enter键重试\n", err)
			if retries >= maxDBRetries {
				fmt.Fprintf(os.Stderr, "\n数据库连接失败！可能是环境或配置问题\n")
				bufio.NewReader(os.Stdin).ReadString('\n')
				os.Exit(1)
			}
			bufio.NewReader(os.Stdin).ReadString('\n')
			continue
		}

		log.Fatalf("error opening database %q: %v", absDB, err)
		return nil
	}
}
