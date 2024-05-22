package main

import (
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"os"
)

// Usage: your_git.sh <command> <arg1> <arg2> ...
func main() {
	// Uncomment this block to pass the first stage!
	//
	if len(os.Args) < 2 {
		ePrintf("usage: mygit <command> [<args>...]\n")
		os.Exit(1)
	}

	switch command := os.Args[1]; command {
	case "init":
		for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				ePrintf("Error creating directory: %s\n", err)
			}
		}

		headFileContents := []byte("ref: refs/heads/main\n")
		if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
			ePrintf("Error writing file: %s\n", err)
		}

		fmt.Println("Initialized git directory")

	case "cat-file":
		if len(os.Args) != 4 {
			ePrintf("usage: mygit cat-file <flag> <file>\n")
			os.Exit(1)
		}
		if os.Args[2] != "-p" {
			ePrintf("usage: mygit cat-file -p <file>\n")
			os.Exit(1)
		}
		objHash := os.Args[3]
		if len(objHash) != 40 {
			ePrintf("invalid object hash: %q", objHash)
			os.Exit(1)
		}
		file, err := os.Open(fmt.Sprintf(".git/objects/%s/%s", objHash[0:2], objHash[2:]))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				ePrintf("no such object: %q", objHash)
				os.Exit(1)
			}
			ePrintf("could not open the object file: %v", err)
			os.Exit(1)
		}
		readObjectFile(file)
		file.Close()

	default:
		ePrintf("Unknown command %s\n", command)
		os.Exit(1)
	}
}

func ePrintf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format, a...)
}

func readObjectFile(r io.Reader) error {
	z, err := zlib.NewReader(r)
	if err != nil {
		return err
	}
	defer z.Close()
	content, err := io.ReadAll(z)
	if err != nil {
		return err
	}
	zeroPos := 0
	for _, by := range content {
		if by == 0 {
			break
		}
		zeroPos++
	}
	fmt.Printf("%s", content[zeroPos+1:])
	return nil
}
