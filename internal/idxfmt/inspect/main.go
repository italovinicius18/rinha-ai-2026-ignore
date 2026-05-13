// inspect prints the header + the first few entries of an index file.
// Diagnostic-only; not shipped in the runtime image.
package main

import (
	"fmt"
	"log"
	"os"

	"rinha2026/solution/internal/idxfmt"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: inspect <index.bin> [n]")
		os.Exit(2)
	}
	n := 5
	if len(os.Args) >= 3 {
		fmt.Sscanf(os.Args[2], "%d", &n)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	r, err := idxfmt.OpenFlat(data)
	if err != nil {
		log.Fatal(err)
	}
	h := r.Header()
	fmt.Printf("magic   : %s\n", "RINH2026")
	fmt.Printf("version : %d\n", h.Version)
	fmt.Printf("format  : %d (FLAT32)\n", h.Format)
	fmt.Printf("count   : %d\n", h.Count)
	fmt.Printf("dim     : %d\n", h.Dim)
	fmt.Printf("crc32   : %#08x\n", h.CRC32)
	fmt.Printf("file    : %d bytes (header 64 + body %d)\n",
		len(data), idxfmt.BodySize(int(h.Count)))
	if n > r.Count() {
		n = r.Count()
	}
	fmt.Printf("\nfirst %d entries:\n", n)
	var nFraud int
	for i := 0; i < r.Count(); i++ {
		if r.IsFraud(i) {
			nFraud++
		}
	}
	for i := 0; i < n; i++ {
		fmt.Printf("  [%d] fraud=%v vec=%v\n", i, r.IsFraud(i), r.Vector(i))
	}
	fmt.Printf("\nlabel summary: %d fraud / %d legit\n", nFraud, r.Count()-nFraud)
}
