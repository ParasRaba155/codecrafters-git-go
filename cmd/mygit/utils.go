package main

import (
	"fmt"
	"io"
	"os"
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

func modeFromGit(gitMode string) os.FileMode {
	switch gitMode {
	case "100644":
		return 0644
	case "100755":
		return 0755
	case "40000":
		return os.ModeDir | 0755
	default:
		return 0644 // fallback
	}
}
