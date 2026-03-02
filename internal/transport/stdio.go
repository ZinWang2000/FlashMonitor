package transport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"flashmonitor/internal/rpc"
)

type StdioTransport struct {
	handler *rpc.Handler
	done    chan struct{}
}

func NewStdioTransport(handler *rpc.Handler) *StdioTransport {
	return &StdioTransport{handler: handler, done: make(chan struct{})}
}

func (t *StdioTransport) Start() error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	for scanner.Scan() {
		select {
		case <-t.done:
			return nil
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		resp := t.handler.Handle(line)
		out, err := json.Marshal(resp)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "marshal error: %v\n", err)
			continue
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s\n", out)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return fmt.Errorf("stdio scanner: %w", err)
	}
	return nil
}

func (t *StdioTransport) Stop() error {
	close(t.done)
	return nil
}
