package main

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
