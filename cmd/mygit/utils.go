package main

import (
	"errors"
	"os"
	"path/filepath"
)

func getFileFromHash(objHash string) *os.File {
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
