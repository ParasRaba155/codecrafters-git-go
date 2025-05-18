package clone

import (
	"errors"
	"fmt"
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
