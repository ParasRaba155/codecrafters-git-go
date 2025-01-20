package main

import (
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// readObjectFile will return the content after the null character byte
func readObjectFile(r io.Reader) ([]byte, error) {
	z, err := zlib.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	content, err := io.ReadAll(z)
	if err != nil {
		return nil, err
	}
	zeroPos := 0
	for _, by := range content {
		if by == 0 {
			break
		}
		zeroPos++
	}
	return content[zeroPos+1:], nil
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
