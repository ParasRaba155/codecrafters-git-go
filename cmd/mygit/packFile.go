package main

type ObjectType byte

const (
	OBJ_INVALID   ObjectType = 0
	OBJ_COMMIT    ObjectType = 1
	OBJ_TREE                 = 2
	OBJ_BLOB                 = 3
	OBJ_TAG                  = 4
	OBJ_OFS_DELTA            = 6
	OBJ_REF_DELTA            = 7
)

// validateObjectType will check whether it is a valid object-type value, if it's not it will return ObjectType 0 for invalid
func validateObjectType(s byte) ObjectType {
	if s > 7 {
		return OBJ_INVALID
	}
	return ObjectType(s)
}
