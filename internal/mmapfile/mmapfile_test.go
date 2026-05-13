package mmapfile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.bin")
	want := []byte("RINH2026\x01\x00\x01\x00..the rest is arbitrary..\xff\x00\xaa")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if m.Len() != len(want) {
		t.Errorf("Len: got %d, want %d", m.Len(), len(want))
	}
	if !bytes.Equal(m.Data(), want) {
		t.Errorf("Data mismatch")
	}
}

func TestOpen_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.bin")
	_ = os.WriteFile(path, nil, 0o644)
	if _, err := Open(path); err == nil {
		t.Error("expected error on empty file")
	}
}

func TestOpen_MissingFile(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "no-such-file.bin")); err == nil {
		t.Error("expected error on missing file")
	}
}

func TestClose_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.bin")
	_ = os.WriteFile(path, []byte("hello"), 0o644)
	m, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
