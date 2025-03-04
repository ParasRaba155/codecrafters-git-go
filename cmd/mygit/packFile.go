package main

import (
	"io"
)

//go:generate stringer -type=ObjectType
type ObjectType byte

const (
	OBJ_INVALID   ObjectType = 0
	OBJ_COMMIT    ObjectType = 1
	OBJ_TREE      ObjectType = 2
	OBJ_BLOB      ObjectType = 3
	OBJ_TAG       ObjectType = 4
	OBJ_OFS_DELTA ObjectType = 6
	OBJ_REF_DELTA ObjectType = 7
)

// convertToObjectType will check whether it is a valid object-type value, if it's not it will return ObjectType 0 for invalid
func convertToObjectType(s byte) ObjectType {
	if s > 7 {
		return OBJ_INVALID
	}
	return ObjectType(s)
}

func (o ObjectType) ToGitType() string {
	switch o {
	case OBJ_TREE:
		return "tree"
	case OBJ_BLOB:
		return "blob"
	case OBJ_COMMIT:
		return "commit"
	case OBJ_TAG:
		return "tag"
	case OBJ_OFS_DELTA:
		return "ofsdelta"
	case OBJ_REF_DELTA:
		return "refdelta"
	default:
		return ""
	}
}

const (
	VarintEncodingBits  uint8 = 7
	VarintContinueFlag  uint8 = 1 << VarintEncodingBits
	TypeBits            uint8 = 3
	TypeByteSizeBits    uint8 = VarintEncodingBits - TypeBits
	CopyInstructionFlag uint8 = 1 << 7
	CopySizeBytes       uint8 = 3
	CopyZeroSize        int   = 0x10000
	CopyOffsetBytes     uint8 = 4
)

func readVarintByte(packfileReader io.Reader) (uint8, bool, error) {
	bytes := make([]byte, 1)
	_, err := packfileReader.Read(bytes)
	if err != nil {
		return 0, false, err
	}

	byteValue := bytes[0]
	value := byteValue & ^VarintContinueFlag
	moreBytes := (byteValue & VarintContinueFlag) != 0

	return value, moreBytes, nil
}

func readSizeEncoding(packfileReader io.Reader) (int, error) {
	var value int
	var length uint

	for {
		byteValue, moreBytes, err := readVarintByte(packfileReader)
		if err != nil {
			return 0, err
		}

		value |= int(byteValue) << length

		if !moreBytes {
			return value, nil
		}

		length += uint(VarintEncodingBits)
	}
}

func keepBits(value int, bits uint8) int {
	return value & ((1 << bits) - 1)
}

func readTypeAndSize(packfileReader io.ReadSeeker) (ObjectType, int, error) {
	value, err := readSizeEncoding(packfileReader)
	if err != nil {
		return OBJ_INVALID, 0, err
	}

	objectType := uint8(keepBits(value>>TypeByteSizeBits, TypeBits))
	size := keepBits(value, TypeByteSizeBits) | (value>>VarintEncodingBits)<<TypeByteSizeBits

	return convertToObjectType(objectType), size, nil
}
