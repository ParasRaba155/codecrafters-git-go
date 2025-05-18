package clone

import "fmt"

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

func (o ObjectType) String() string {
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
		return fmt.Sprintf("invalid(%d))", o)
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
