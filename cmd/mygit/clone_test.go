package main

import (
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
