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
	_, err = validatePacketFile(packetFile)
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
	pktLines, err := validatePacketFile(packetFile)
	if err != nil {
		t.Fatalf("testdata validatePacketFile: %s", err)
	}
	refs, err := getAllRefs(pktLines)
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
