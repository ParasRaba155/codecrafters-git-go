package common

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// FormatGitObjectContent constructs content in Git object storage format.
//
// The Git object storage format consists of the following structure:
//
//	<type> <content_length><null_byte><content>
//
// where:
// - <type> is a string representing the type of the object (e.g., "blob", "tree").
// - <content_length> is the size of the content in bytes.
// - <null_byte> is a null byte (`\0`) separating the metadata from the content.
// - <content> is the actual data of the object.
//
// Example:
//
//	content := []byte("hello world")
//	formattedContent := CreateContentWithInfo("blob", content)
//	fmt.Printf("%s\n", formattedContent)
func FormatGitObjectContent(typ string, content []byte) []byte {
	contentLength := len(content)
	contentDigitLength := numOfDigits(contentLength)

	result := make([]byte, 0, len(typ)+1+contentDigitLength+1+len(content))
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

// WriteCompactContent writes the `content` to `w` with zlib compression
func WriteCompactContent(w io.Writer, content io.Reader) error {
	z := zlib.NewWriter(w)
	defer z.Close()

	contentByte, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("WriteCompactContent file could not read the content: %s", err)
	}

	n, err := z.Write(contentByte)
	if err != nil {
		return fmt.Errorf("WriteCompactContent file could not write the content: %s", err)
	}
	if n != len(contentByte) {
		return fmt.Errorf(
			"WriteCompactContent content length and written bytes do not match %d and %d",
			len(contentByte),
			n,
		)
	}
	return nil
}

// CreateEmptyObjectFile will crete hash[0:2],hash[2:40]
func CreateEmptyObjectFile(baseDir, hash string) (*os.File, error) {
	if len(hash) != 40 {
		return nil, fmt.Errorf("invalid length of sha object: %d", len(hash))
	}
	dir := filepath.Join(baseDir, ".git", "objects", hash[:2])
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil && !os.IsExist(err) {
		return nil, err
	}
	objPath := filepath.Join(dir, hash[2:])
	return os.Create(objPath)
}

// GetFileFromHash splits the hash into git object format
//
// e.g. "23abcdefgh...." -> ./git/objects/23/<remaniing_38_chars>
func GetFileFromHash(basdir, objHash string) (*os.File, error) {
	if len(objHash) != 40 {
		return nil, fmt.Errorf("invalid object hash: %q", objHash)
	}
	dir, rest := objHash[0:2], objHash[2:]
	path := filepath.Join(basdir, ".git", "objects", dir, rest)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no such object: %q & %q", objHash, path)
		}
		return nil, fmt.Errorf("could not open the object file %q: %w", objHash, err)
	}
	return file, nil
}

// ReadObjectFile will return the content after the null character byte
// and the type of the content e.g. the "tree", "blog", etc.
func ReadObjectFile(r io.Reader) ([]byte, string, error) {
	content, err := ReadCompressed(r)
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

// ReadCompressed reads the whole reader using zlib decompress
func ReadCompressed(r io.Reader) ([]byte, error) {
	zlibReader, err := zlib.NewReader(r)
	if err != nil {
		full, err2 := io.ReadAll(r)
		fmt.Println(err2)
		fmt.Println("----------------DEBUG-------------------------------------")
		fmt.Printf("%s", full)
		fmt.Println("----------------DEBUG-------------------------------------")
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
