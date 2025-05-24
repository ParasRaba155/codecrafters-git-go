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

// StringToObjectType will return OBJ_INVALID  on invalid object
// It is caller's responsibility to check for that
func StringToObjectType(str string) GitObjectType {
	switch str {
	case "tree":
		return OBJ_TREE
	case "blob":
		return OBJ_BLOB
	case "commit":
		return OBJ_COMMIT
	case "tag":
		return OBJ_TAG
	case "ofsdelta":
		return OBJ_OFS_DELTA
	case "refdelta":
		return OBJ_REF_DELTA
	default:
		return OBJ_INVALID
	}
}

// GitObject when we unpack a pack file we will get a git object type
type GitObject struct {
	ObjectType GitObjectType
	// Size the decompressed size
	Size    int
	Content []byte
	// Base would be hash of the base object in case of DELTA objects
	Base string
}
