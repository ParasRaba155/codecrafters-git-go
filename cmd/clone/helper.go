package clone

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
)

var errInvalidSize = errors.New("invalid size")

func readBigEndian(b [4]byte) uint32 {
	return uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
}

// packObjectSize will read the object size according to the git specification of
// object sizes in [git documentation](https://git-scm.com/docs/gitformat-pack)
//
// NOTE: it is expected that the content provided will be the body of the pack file
func packObjectSize(content []byte) (length uint64, objType GitObjectType, bytesRead int, err error) {
	if len(content) == 0 {
		return 0, 0, 0, fmt.Errorf("%w: no content", errInvalidSize)
	}
	b := content[bytesRead]
	objType = GitObjectType((b >> 4) & 0x07)
	if objType > 7 || objType == 5 {
		objType = OBJ_INVALID
	}
	sizeFromFirstByte := b & 0x0F
	length += uint64(sizeFromFirstByte)
	bytesRead++

	more := ((b >> 7) & 1) == 1
	if !more {
		return uint64(sizeFromFirstByte), objType, 1, nil
	}

	bitShift := 4
	for more {
		if bytesRead >= len(content) || bitShift >= 64 {
			return 0, 0, 0, errors.New("unexpected end of content")
		}
		b := content[bytesRead]
		// formula is for every new byte 2 ^ bitshift
		// byteshift starts with 4 bits, then 4 + 7 (as 1st bit is for more size)
		additonalLength := uint64(b&0x7F) * (1 << bitShift)
		length += uint64(additonalLength)
		more = ((b >> 7) & 1) == 1
		bytesRead++
		bitShift += 7
	}
	return length, objType, bytesRead, nil
}

// findAndDecompress: Search for a valid zlib stream in raw byte content and decompress it
func findAndDecompress(data []byte) (compressed []byte, decompressed []byte, used int, err error) {
	// NOTE: we specifically use the bytes.NewReader because it implements
	// [io.ByteReader](https://pkg.go.dev/io#ByteReader)
	// which will ensure that only the compressed data is read
	// We could also go with [bytes.NewBuffer](https://pkg.go.dev/bytes#NewBuffer) as [bytes.Buffer](https://pkg.go.dev/bytes#Buffer)
	// also implements the [io.ByteReader](https://pkg.go.dev/io#ByteReader)
	reader := bytes.NewReader(data)
	zlibReader, err := zlib.NewReader(reader)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("creating zlib reader: %w", err)
	}
	defer zlibReader.Close()

	decompressed, err = io.ReadAll(zlibReader)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("reading uncompressed data: %w", err)
	}

	// The crucial part: How many bytes did the zlib.Reader actually consume from 'reader'?
	// The bytes.NewReader keeps track of its current position.
	// Total size - remaining bytes = bytes read
	used = int(reader.Size()) - reader.Len()

	compressed = data[:used]

	return compressed, decompressed, used, nil
}
