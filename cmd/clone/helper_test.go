package clone

import (
	"os"
	"testing"
)

// TestDecodeLength tests the packObjectSize function with various scenarios.
func TestDecodeLength(t *testing.T) {
	// Test Case 1: Single byte (MSB is 0)
	t.Run("Single Byte", func(t *testing.T) {
		input := []byte{0x0a} // 00001010
		expectedLength := uint64(10)
		expectedObjectType := OBJ_INVALID // Assuming the first 3 bits are for object type
		expectedBytesRead := 1

		length, objType, bytesRead, err := packObjectSize(input)
		if err != nil {
			t.Errorf("packObjectSize() error = %v, expected nil", err)
			return
		}
		if length != expectedLength {
			t.Errorf("packObjectSize() length = %d, expected %d", length, expectedLength)
		}
		if objType != expectedObjectType {
			t.Errorf("packObjectSize() objType = %s, expected %s", objType, expectedObjectType)
		}
		if bytesRead != expectedBytesRead {
			t.Errorf("packObjectSize() bytesRead = %d, expected %d", bytesRead, expectedBytesRead)
		}
	})

	// Test Case 2: Two bytes (MSB of first byte is 1, MSB of second is 0)
	t.Run("Two Bytes", func(t *testing.T) {
		input := []byte{0x8a, 0x42} // 10001010 01000010
		expectedLength := uint64(1066)
		expectedObjectType := OBJ_INVALID // 0x8a >> 4 & 0x7 = 8 >> 4 & 0x7 = 0010 (binary) = 2
		expectedBytesRead := 2

		length, objType, bytesRead, err := packObjectSize(input)
		if err != nil {
			t.Errorf("packObjectSize() error = %v, expected nil", err)
			return
		}
		if length != expectedLength {
			t.Errorf("packObjectSize() length = %d, expected %d", length, expectedLength)
		}
		if objType != expectedObjectType {
			t.Errorf("packObjectSize() objType = %s, expected %s", objType, expectedObjectType)
		}
		if bytesRead != expectedBytesRead {
			t.Errorf("packObjectSize() bytesRead = %d, expected %d", bytesRead, expectedBytesRead)
		}
	})

	// Test Case 3: Three bytes (MSBs are 1, 1, 0)
	t.Run("Three Bytes", func(t *testing.T) {
		input := []byte{0x82, 0x81, 0x03} // 10000010 10000001 00000011
		expectedLength := uint64(6162)
		expectedObjectType := OBJ_INVALID
		expectedBytesRead := 3

		length, objType, bytesRead, err := packObjectSize(input)
		if err != nil {
			t.Errorf("packObjectSize() error = %v, expected nil", err)
			return
		}
		if length != expectedLength {
			t.Errorf("packObjectSize() length = %d, expected %d", length, expectedLength)
		}
		if objType != expectedObjectType {
			t.Errorf("packObjectSize() objType = %s, expected %s", objType, expectedObjectType)
		}
		if bytesRead != expectedBytesRead {
			t.Errorf("packObjectSize() bytesRead = %d, expected %d", bytesRead, expectedBytesRead)
		}
	})

	// Test Case 4: Example from Documentation
	t.Run("Documentation Example", func(t *testing.T) {
		input := []byte{0x96, 0x0a} // 10010110 00001010
		expectedLength := uint64(166)
		expectedObjectType := OBJ_COMMIT
		expectedBytesRead := 2

		length, objType, bytesRead, err := packObjectSize(input)
		if err != nil {
			t.Errorf("packObjectSize() error = %v, expected nil", err)
			return
		}
		if length != expectedLength {
			t.Errorf("packObjectSize() length = %d, expected %d", length, expectedLength)
		}
		if objType != expectedObjectType {
			t.Errorf("packObjectSize() objType = %s, expected %s", objType, expectedObjectType)
		}
		if bytesRead != expectedBytesRead {
			t.Errorf("packObjectSize() bytesRead = %d, expected %d", bytesRead, expectedBytesRead)
		}
	})

	// Test Case 5: Zero Length
	t.Run("Zero Length", func(t *testing.T) {
		input := []byte{0x00}
		expectedLength := uint64(0)
		expectedObjectType := OBJ_INVALID // Assuming 0 for the object type
		expectedBytesRead := 1

		length, objType, bytesRead, err := packObjectSize(input)
		if err != nil {
			t.Errorf("packObjectSize() error = %v, expected nil", err)
			return
		}
		if length != expectedLength {
			t.Errorf("packObjectSize() length = %d, expected %d", length, expectedLength)
		}
		if objType != expectedObjectType {
			t.Errorf("packObjectSize() objType = %s, expected %s", objType, expectedObjectType)
		}
		if bytesRead != expectedBytesRead {
			t.Errorf("packObjectSize() bytesRead = %d, expected %d", bytesRead, expectedBytesRead)
		}
	})

	// Test Case 6: Maximum length that can be stored in 9 bytes.
	// This test case is commented out in the original, so I'll leave it commented,
	// but if uncommented, the expectedObjectType would need to be determined
	// based on the first byte's high bits.
	// t.Run("Max Length", func(t *testing.T) {
	// 	input := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}
	// 	expectedLength := uint64(0x7ffffffffffffffff)
	// 	expectedObjectType := GitObjectType(...) // Determine based on 0xff
	// 	expectedBytesRead := 9

	// 	length, objType, bytesRead, err := packObjectSize(input)
	// 	if err != nil {
	// 		t.Errorf("packObjectSize() error = %v, expected nil", err)
	// 		return
	// 	}
	// 	if length != expectedLength {
	// 		t.Errorf("packObjectSize() length = %d, expected %d", length, expectedLength)
	// 	}
	// 	if objType != expectedObjectType {
	// 		t.Errorf("packObjectSize() objType = %s, expected %s", objType, expectedObjectType)
	// 	}
	// 	if bytesRead != expectedBytesRead {
	// 		t.Errorf("packObjectSize() bytesRead = %d, expected %d", bytesRead, expectedBytesRead)
	// 	}
	// })

	// Test Case 8: Empty input
	t.Run("Empty Input", func(t *testing.T) {
		input := []byte{}
		_, _, _, err := packObjectSize(input)
		if err == nil {
			t.Errorf("packObjectSize() error = nil, expected error")
			return
		}
	})
}

func TestReadPackFile(t *testing.T) {
	content, err := os.ReadFile("../../testdata/pack-response.txt")
	if err != nil {
		t.Errorf("error in reading packfile: %v", err)
	}
	err = ReadPackFile(content)
	if err != nil {
		t.Errorf("error in reading packfile: %v", err)
	}
}
