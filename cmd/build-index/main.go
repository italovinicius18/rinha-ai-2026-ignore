// build-index decompresses (if needed) and streams a references JSON file
// into the on-disk index format defined in internal/idxfmt.
//
// Usage:
//
//	build-index --in=resources/references.json.gz --out=/data/index.bin [--format=flat|int8]
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"time"

	"rinha2026/solution/internal/idxfmt"
)

type entry struct {
	Vector [idxfmt.Dim]float64 `json:"vector"`
	Label  string              `json:"label"`
}

// writer abstracts over FlatWriter and Int8Writer so the streaming loop is shared.
type writer interface {
	Add(v [idxfmt.Dim]float64, fraud bool) error
	Close() error
}

func main() {
	in := flag.String("in", "", "input references file (.json or .json.gz; gzip auto-detected)")
	out := flag.String("out", "", "output index file")
	format := flag.String("format", "flat", "output format: flat (float32) or int8")
	flag.Parse()

	if *in == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	switch *format {
	case "flat", "int8":
	default:
		log.Fatalf("unsupported format %q (choose flat or int8)", *format)
	}

	if err := run(*in, *out, *format); err != nil {
		log.Fatal(err)
	}
}

func run(inPath, outPath, format string) error {
	start := time.Now()
	f, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer f.Close()

	br := bufio.NewReaderSize(f, 1<<20)
	var r io.Reader = br
	hdr, err := br.Peek(2)
	if err != nil {
		return fmt.Errorf("peek input: %w", err)
	}
	if hdr[0] == 0x1f && hdr[1] == 0x8b {
		gz, gerr := gzip.NewReader(br)
		if gerr != nil {
			return fmt.Errorf("gzip: %w", gerr)
		}
		defer gz.Close()
		r = gz
		log.Printf("input is gzipped — streaming through gzip.Reader")
	}

	outF, err := os.Create(outPath)
	if err != nil {
		return err
	}
	var w writer
	if format == "int8" {
		w, err = idxfmt.NewInt8Writer(outF)
	} else {
		w, err = idxfmt.NewFlatWriter(outF)
	}
	if err != nil {
		_ = outF.Close()
		return err
	}

	dec := json.NewDecoder(r)
	dec.UseNumber() // not strictly needed, but cheap

	t, err := dec.Token()
	if err != nil {
		return fmt.Errorf("first token: %w", err)
	}
	d, ok := t.(json.Delim)
	if !ok || d != '[' {
		return errors.New("expected JSON array at top level")
	}

	var count uint64
	var nFraud uint64
	const progressEvery = uint64(250_000)
	nextLog := progressEvery
	for dec.More() {
		var e entry
		if err := dec.Decode(&e); err != nil {
			return fmt.Errorf("decode entry %d: %w", count, err)
		}
		fraud := e.Label == "fraud"
		if err := w.Add(e.Vector, fraud); err != nil {
			return fmt.Errorf("write entry %d: %w", count, err)
		}
		if fraud {
			nFraud++
		}
		count++
		if count >= nextLog {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			log.Printf("  %d entries · %s elapsed · heap=%dMB",
				count, time.Since(start).Round(time.Millisecond), ms.HeapInuse>>20)
			nextLog += progressEvery
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("trailing token: %w", err)
	}
	if err := w.Close(); err != nil {
		_ = outF.Close()
		return fmt.Errorf("writer close: %w", err)
	}
	if err := outF.Close(); err != nil {
		return err
	}

	fi, err := os.Stat(outPath)
	if err != nil {
		return err
	}
	log.Printf("done · %d entries (%d fraud / %d legit) · output=%dB · %s",
		count, nFraud, count-nFraud, fi.Size(), time.Since(start).Round(time.Millisecond))
	return nil
}
