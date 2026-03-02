package logger

import (
	"testing"
)

func TestRingLogger(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	l := Global()
	t.Cleanup(func() { l.Close() })

	Info("hello")
	Warn("world")
	Errorf("error %d", 42)

	entries := l.GetLast(10)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[2].Message != "error 42" {
		t.Errorf("expected 'error 42', got %q", entries[2].Message)
	}
}

func TestRingOverflow(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	l := Global()
	t.Cleanup(func() { l.Close() })

	for i := 0; i < ringSize+100; i++ {
		Infof("msg %d", i)
	}
	entries := l.GetLast(0)
	if len(entries) != ringSize {
		t.Errorf("expected %d entries, got %d", ringSize, len(entries))
	}
}
