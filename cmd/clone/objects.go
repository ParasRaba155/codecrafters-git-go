package clone

import "fmt"

type GitObjectType byte

const (
	OBJ_INVALID   GitObjectType = 0
	OBJ_COMMIT    GitObjectType = 1
	OBJ_TREE      GitObjectType = 2
	OBJ_BLOB      GitObjectType = 3
	OBJ_TAG       GitObjectType = 4
	OBJ_OFS_DELTA GitObjectType = 6
	OBJ_REF_DELTA GitObjectType = 7
)

func (o GitObjectType) String() string {
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

type GitObject struct {
	ObjectType GitObjectType
	Size       int
	Content    []byte
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
