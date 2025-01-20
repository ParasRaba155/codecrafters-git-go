package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
		defer file.Close()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				ePrintf("no such object: %q", objHash)
				os.Exit(1)
			}
			ePrintf("could not open the object file: %v", err)
			os.Exit(1)
		}
		err = readObjectFile(file)
		if err != nil {
			ePrintf("error in reading the object file: %s", err)
			os.Exit(1)
		}

	case "hash-object":
		if len(os.Args) != 4 {
			ePrintf("usage: mygit cat-file <flag> <file>\n")
			os.Exit(1)
		}
		if os.Args[2] != "-w" {
			ePrintf("usage: mygit cat-file -p <file>\n")
			os.Exit(1)
		}
		file, err := os.Open(os.Args[3])
		if err != nil {
			ePrintf("error in opening the given file: %s", err)
			os.Exit(1)
		}
		defer file.Close()
		fileContent, err := io.ReadAll(file)
		if err != nil {
			ePrintf("error in reading the given file: %s", err)
			os.Exit(1)
		}
		contentToWrite := createContentWithInfo("blob", fileContent)
		fileSHA, err := calculateSHA(contentToWrite)
		if err != nil {
			ePrintf("error in calculating the SHA: %s", err)
			os.Exit(1)
		}
		nFile, err := createEmptyObjectFile(fileSHA)
		if err != nil {
			ePrintf("error in creating the object file: %s", err)
			os.Exit(1)
		}
		err = createObjectFile(nFile, bytes.NewReader(contentToWrite))
		if err != nil {
			ePrintf("error in writing to the object file: %s", err)
			os.Exit(1)
		}
		fmt.Printf("%s\n", fileSHA)

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

func createObjectFile(w io.Writer, content io.Reader) error {
	z := zlib.NewWriter(w)
	defer z.Close()

	contentByte, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("createObjectFile file could not read the content: %s", err)
	}

	n, err := z.Write(contentByte)
	if err != nil {
		return fmt.Errorf("createObjectFile file could not write the content: %s", err)
	}
	if n != len(contentByte) {
		return fmt.Errorf("createObjectFile content length and written bytes do not match %d and %d", len(contentByte), n)
	}
	return nil
}

func calculateSHA(content []byte) (string, error) {
	hasher := sha1.New()
	n, err := hasher.Write(content)
	if err != nil {
		return "", err
	}
	if n != len(content) {
		return "", fmt.Errorf("mismatch in the bytes written and content: %d and %d", n, len(content))
	}
	sha := hex.EncodeToString(hasher.Sum(nil))
	return sha, nil
}

func createEmptyObjectFile(sha string) (*os.File, error) {
	if len(sha) != 40 {
		return nil, fmt.Errorf("invalid length of sha object: %d", len(sha))
	}
	dir, rest := sha[0:2], sha[2:]
	err := os.Mkdir(fmt.Sprintf("./.git/objects/%s", dir), fs.FileMode(os.ModeDir))
	if err != nil {
		return nil, err
	}
	return os.Create(fmt.Sprintf("./.git/objects/%s/%s", dir, rest))
}

func createContentWithInfo(typ string, content []byte) []byte {
	contentLength := len(content)
	contentDigitLength := numOfDigits(contentLength)

	result := make([]byte, 0, len(typ)+contentLength+1+contentDigitLength+len(content))
	// append type
	result = append(result, typ...)
	// append the space
	result = append(result, ' ')
	// append the size
	result = append(result, []byte(fmt.Sprintf("%d", contentLength))...)
	// append the null byte
	result = append(result, 0)
	// append the content
	result = append(result, content...)
	return result
}

func numOfDigits(a int) int {
	count := 0
	for a != 0 {
		a /= 10
		count++
	}
	return count
}
