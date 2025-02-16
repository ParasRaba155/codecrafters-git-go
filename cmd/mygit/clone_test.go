package main

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

func Test_ValidatePacketFile(t *testing.T) {
	packetFile, err := os.Open("../../testdata/code-crafter-response.txt")
	if err != nil {
		t.Fatalf("testdata file open: %s", err)
	}
	defer packetFile.Close()
	_, err = ParsePacketFile(packetFile)
	if err != nil {
		t.Fatalf("testdata validatePacketFile: %s", err)
	}
}

func Test_ValidateHEAD_Ref(t *testing.T) {
	packetFile, err := os.Open("../../testdata/code-crafter-response.txt")
	if err != nil {
		t.Fatalf("testdata file open: %s", err)
	}
	defer packetFile.Close()
	pktLines, err := ParsePacketFile(packetFile)
	if err != nil {
		t.Fatalf("testdata validatePacketFile: %s", err)
	}
	refs, err := RefRecordsFromPacketLines(pktLines)
	if err != nil {
		t.Fatalf("get refs: %s", err)
	}
	if len(refs) == 0 {
		t.Fatalf("no refs found")
	}
	if refs[0].Name != "HEAD" {
		t.Fatalf("HEAD ref not found: %q", refs[0].Name)
	}
}

func Test_ValidatePackFile(t *testing.T) {
	packFile, err := os.Open("../../testdata/pack-response.txt")
	if err != nil {
		t.Fatalf("open the pack response: %s", err)
	}
	defer packFile.Close()
	err = ParseDiscoverRefResponse(packFile)
	if err != nil {
		t.Fatalf("validate the pack file header: %s", err)
	}
}

func Test_ReadPackFileHeader(t *testing.T) {
	inputs := [...][]byte{
		{0b01100010},                         // Type 3, Size 2 (No extra size bytes)
		{0b10100011, 0b00010101},             // Type 5, Size 0b000101010011 (339 in decimal)
		{0b00100111},                         // Type 1, Size 7 (No extra size bytes)
		{0b11100001, 0b10000001, 0b00000010}, // Type 7, Size 0b0000001000000001 (257 in decimal)
		{0b10101100, 0b01111111},             // Type 5, Size 0b01111111001100 (2044 in decimal)
	}

	testCases := [...]struct {
		input            []byte
		outputSize       int
		outputObjectType ObjectType
	}{
		{
			input:            inputs[0],
			outputSize:       2,
			outputObjectType: OBJ_OFS_DELTA,
		},
		{
			input:            inputs[1],
			outputSize:       339,
			outputObjectType: OBJ_TREE,
		},
		{
			input:            inputs[2],
			outputSize:       7,
			outputObjectType: OBJ_TREE,
		},
		{
			input:            inputs[3],
			outputSize:       4113,
			outputObjectType: OBJ_OFS_DELTA,
		},
		{
			input:            inputs[4],
			outputSize:       2044,
			outputObjectType: OBJ_TREE,
		},
	}

	for i := range testCases {
		t.Run(fmt.Sprintf("test case: %d", i), func(t *testing.T) {
			reader := bytes.NewReader(testCases[i].input)
			objType, size, err := readPackFileHeader(reader)
			if err != nil {
				t.Errorf("did not expected error, got: %s", err)
			}
			if size != testCases[i].outputSize {
				t.Errorf("expected %d size got %d", testCases[i].outputSize, size)
			}
			if objType != testCases[i].outputObjectType {
				t.Errorf("expected %d object got %d", testCases[i].outputObjectType, objType)
			}
		})
	}
}
