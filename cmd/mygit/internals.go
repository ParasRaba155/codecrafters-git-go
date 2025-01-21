package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
)

type GitTree struct {
	Mode os.FileMode
	Name string
	// SHA is the actual SHA of the file without the hex encoding
	SHA [20]byte
}

// readObjectFile will return the content after the null character byte
// and the type of the content e.g. the "tree", "blog", etc.
func readObjectFile(r io.Reader) ([]byte, string, error) {
	z, err := zlib.NewReader(r)
	if err != nil {
		return nil, "", err
	}
	defer z.Close()
	content, err := io.ReadAll(z)
	if err != nil {
		return nil, "", err
	}
	zeroPos := 0
	for _, by := range content {
		if by == 0 {
			break
		}
		zeroPos++
	}
	parts := bytes.Split(content[:zeroPos], []byte{' '})
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("couldn't find the object type")
	}
	return content[zeroPos+1:], string(parts[0]), nil
}

// createObjectFile writes the content byte to w with zlib compression
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

// calculateSHA will calculate the SHA256
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

func getRawSHA(sha string) ([]byte, error) {
	dst := make([]byte, 20)
	n, err := hex.Decode(dst, []byte(sha))
	if err != nil {
		return nil, fmt.Errorf("couldn't decode into hex: %w", err)
	}
	if n != len(dst) {
		return nil, fmt.Errorf("couldn't decode fully with decoded byte: %d and total byte: %d", n, len(dst))
	}
	return dst, nil
}

// createEmptyObjectFile will crete sha[0:2],sha[2:40]
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

// createContentWithInfo
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

// readATreeObject unmarshal the byte array into GitTree object
// it is expected that the header would already been stripped from the content
// and we are indeed only getting the body of the tree object
func readATreeObject(content []byte) ([]GitTree, error) {
	// a tree object is of the form
	//// tree <size>\0
	//// <mode> <name>\0<20_byte_sha>
	//// <mode> <name>\0<20_byte_sha>
	result := []GitTree{}

	beforeSpace := 0
	beforeName := 0
	for i := 0; i < len(content); i++ {
		curr := GitTree{}
		if content[i] == ' ' {
			fileMode := content[beforeSpace:i]
			mode, err := strconv.Atoi(string(fileMode))
			if err != nil {
				return nil, err
			}
			curr.Mode = fs.FileMode(mode)
			beforeName = i + 1
		}
		if content[i] == 0 {
			// Extract name
			name := content[beforeName:i]

			// Ensure there are at least 20 bytes for the SHA
			if i+1+20 > len(content) {
				return nil, fmt.Errorf("unexpected end of content while reading SHA")
			}

			// Extract and copy the SHA
			var sha [20]byte
			copy(sha[:], content[i+1:i+1+20])

			curr.Name = string(name)
			curr.SHA = sha

			// Move to the next entry
			beforeSpace = i + 21
			i += 20 // Skip over the SHA bytes
			result = append(result, curr)
		}
	}
	return result, nil
}
