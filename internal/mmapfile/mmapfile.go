// Package mmapfile is a thin wrapper around syscall.Mmap that returns a
// read-only []byte view of a file. Closing the File munmaps the region.
//
// We use MAP_SHARED so the kernel's page cache can deduplicate pages across
// processes that mmap the same on-disk inode — exactly the property we need
// to fit the 168 MB FLAT32 index inside the 350 MB total budget when both
// API replicas mount the same shared volume.
package mmapfile

import (
	"fmt"
	"os"
	"syscall"
)

// File is a memory-mapped file. Data is valid until Close.
type File struct {
	data []byte
	f    *os.File
}

// Open memory-maps path read-only and returns a File.
func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	size := fi.Size()
	if size == 0 {
		_ = f.Close()
		return nil, fmt.Errorf("mmapfile: %s is empty", path)
	}
	if int64(int(size)) != size {
		_ = f.Close()
		return nil, fmt.Errorf("mmapfile: %s too large for int (%d bytes)", path, size)
	}
	data, err := syscall.Mmap(int(f.Fd()), 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("mmap %s: %w", path, err)
	}
	return &File{data: data, f: f}, nil
}

// Data returns the mapped bytes. Slice is valid until Close.
func (m *File) Data() []byte { return m.data }

// Len returns the mapped size in bytes.
func (m *File) Len() int { return len(m.data) }

// Close munmaps and closes the underlying file. Safe to call once.
func (m *File) Close() error {
	if m.data == nil {
		return nil
	}
	err := syscall.Munmap(m.data)
	m.data = nil
	if cerr := m.f.Close(); err == nil {
		err = cerr
	}
	return err
}
