package common

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
)

var defaultHasher = sha1.New()

// CalculateSHA returns the raw 20 byte sha of the given content
func CalculateSHA(content []byte) ([20]byte, error) {
	defer func() { defaultHasher.Reset() }()
	n, err := defaultHasher.Write(content)
	if err != nil {
		return [20]byte{}, err
	}
	if n != len(content) {
		return [20]byte{}, fmt.Errorf(
			"mismatch in the bytes written and content: %d and %d",
			n,
			len(content),
		)
	}
	res := defaultHasher.Sum(nil)
	if len(res) != 20 {
		return [20]byte{}, fmt.Errorf("malformed hash created with '%d' bytes", len(res))
	}
	return [20]byte(res), nil
}

// CalculateEncodedSHA returns the 40 character hex encoded string of the hash of the given content
func CalculateEncodedSHA(content []byte) (string, error) {
	shaBytes, err := CalculateSHA(content)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(shaBytes[:]), nil
}
