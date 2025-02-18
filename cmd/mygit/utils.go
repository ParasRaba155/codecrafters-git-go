package main

import (
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// errWriter is the helper func for writing
type errWriter struct {
	w   io.Writer
	err error
}

// write method calls the internal write method of the w
// it does not write if there is some previous error in the ew
func (ew *errWriter) write(buf []byte) {
	if ew.err != nil {
		return
	}
	n, err := ew.w.Write(buf)
	if len(buf) != n {
		ew.err = fmt.Errorf("to be written: %d, wrote %d", len(buf), n)
	}
	ew.err = err
}

// GetFileFromHash splits the hash into git object format
//
// e.g. "23abcdefgh...." -> ./git/objects/23/<remaniing_28_chars>
func GetFileFromHash(objHash string) *os.File {
	if len(objHash) != 40 {
		ePrintf("invalid object hash: %q", objHash)
		os.Exit(1)
	}
	dir, rest := objHash[0:2], objHash[2:]
	path := filepath.Join(".git/objects", dir, rest)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			ePrintf("no such object: %q", objHash)
			os.Exit(1)
		}
		ePrintf("could not open the object file: %v", err)
		os.Exit(1)
	}
	return file
}

// GetIntFromBigIndian converts to 4 byte Big-Endian to a uint32
func GetIntFromBigIndian(bytes [4]byte) uint32 {
	return uint32(bytes[0])<<24 | uint32(bytes[1])<<16 | uint32(bytes[2])<<8 | uint32(bytes[3])<<0
}

// ReadCompressed reads the whole reader using zlib decompress
func ReadCompressed(r io.Reader) ([]byte, error) {
	zlibReader, err := zlib.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("read compressed: create zlib reader: %w", err)
	}
	defer func() {
		err := zlibReader.Close()
		if err != nil {
			log.Printf("[WARN] ReadCompressed  zlib reader closing: %v", err)
		}
	}()
	decompressedContent, err := io.ReadAll(zlibReader)
	if err != nil {
		return nil, fmt.Errorf("read compressed data: %w", err)
	}
	return decompressedContent, nil
}
