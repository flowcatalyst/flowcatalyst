package runtimes

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

// Caps on reading a jar's describe document. Nothing is executed and nothing
// but the one describe entry is ever decompressed, so these are the whole of
// the zip-bomb defence.
const (
	// JarDescribePath is the jar entry holding the describe document.
	JarDescribePath = "META-INF/flowcatalyst/function.json"
	// MaxJarDescribeBytes caps the describe entry's uncompressed size.
	MaxJarDescribeBytes = 1 << 20
	// MaxJarEntries caps the entries in the jar's central directory.
	MaxJarEntries = 50_000
)

// DescribeJar reads and validates a jvm function's describe document from a
// jar without executing anything: the entry JarDescribePath is the describe
// document (internal/functions/abi) plus a `jvm` block naming the entrypoint
// class. It returns the document as stored. Every failure is an
// *abi.DescribeError, so publish reports it like a bad Wasm describe.
func DescribeJar(jar []byte) ([]byte, error) {
	bad := func(format string, args ...any) error {
		return &abi.DescribeError{Problems: []string{fmt.Sprintf(format, args...)}}
	}
	zr, err := zip.NewReader(bytes.NewReader(jar), int64(len(jar)))
	if err != nil {
		return nil, bad("the artifact is not a jar (zip) file: %v", err)
	}
	if len(zr.File) > MaxJarEntries {
		return nil, bad("the jar has %d entries (at most %d)", len(zr.File), MaxJarEntries)
	}
	var entry *zip.File
	for _, f := range zr.File {
		if f.Name == JarDescribePath {
			entry = f
			break
		}
	}
	if entry == nil {
		return nil, bad("the jar has no %s", JarDescribePath)
	}
	if entry.UncompressedSize64 > MaxJarDescribeBytes {
		return nil, bad("%s is %d bytes uncompressed (at most %d)", JarDescribePath, entry.UncompressedSize64, MaxJarDescribeBytes)
	}
	rc, err := entry.Open()
	if err != nil {
		return nil, bad("%s cannot be read: %v", JarDescribePath, err)
	}
	defer func() { _ = rc.Close() }()
	// The header's size may lie: read one byte past the cap.
	doc, err := io.ReadAll(io.LimitReader(rc, MaxJarDescribeBytes+1))
	if err != nil {
		return nil, bad("%s cannot be read: %v", JarDescribePath, err)
	}
	if len(doc) > MaxJarDescribeBytes {
		return nil, bad("%s is over %d bytes uncompressed", JarDescribePath, MaxJarDescribeBytes)
	}
	d, err := abi.ParseDescribe(doc)
	if err != nil {
		return nil, err
	}
	if d.JVM == nil || d.JVM.Entrypoint == "" {
		return nil, bad("jvm.entrypoint: required (the class implementing the function interface)")
	}
	return doc, nil
}
