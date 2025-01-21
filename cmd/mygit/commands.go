package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

// initCMD has the logic for the init subcommand
func initCMD() {
	for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			ePrintf("Error creating directory: %s\n", err)
		}
	}

	headFileContents := []byte("ref: refs/heads/main\n")
	if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
		ePrintf("Error writing file: %s\n", err)
	}

	fmt.Println("Initialized git directory")
}

// catFileCmd has the logic for the cat-file subcommand
func catFileCmd() {
	if len(os.Args) != 4 {
		ePrintf("usage: mygit cat-file <flag> <file>\n")
		os.Exit(1)
	}
	if os.Args[2] != "-p" {
		ePrintf("usage: mygit cat-file -p <file>\n")
		os.Exit(1)
	}
	objHash := os.Args[3]
	if len(objHash) != 40 {
		ePrintf("invalid object hash: %q", objHash)
		os.Exit(1)
	}
	file, err := os.Open(fmt.Sprintf(".git/objects/%s/%s", objHash[0:2], objHash[2:]))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			ePrintf("no such object: %q", objHash)
			os.Exit(1)
		}
		ePrintf("could not open the object file: %v", err)
		os.Exit(1)
	}
	defer file.Close()
	content, objectType, err := readObjectFile(file)
	if err != nil {
		ePrintf("error in reading the object file: %s", err)
		os.Exit(1)
	}
	if objectType != "blob" {
		ePrintf("the given hash object is not of type \"blob\" is %q", objectType)
	}
	fmt.Printf("%s", content)
}

// hashObjectCmd has the logic for the hash-object subcommand
func hashObjectCmd() {
	if len(os.Args) != 4 {
		ePrintf("usage: mygit cat-file <flag> <file>\n")
		os.Exit(1)
	}
	if os.Args[2] != "-w" {
		ePrintf("usage: mygit cat-file -p <file>\n")
		os.Exit(1)
	}
	file, err := os.Open(os.Args[3])
	if err != nil {
		ePrintf("error in opening the given file: %s", err)
		os.Exit(1)
	}
	defer file.Close()
	fileContent, err := io.ReadAll(file)
	if err != nil {
		ePrintf("error in reading the given file: %s", err)
		os.Exit(1)
	}
	contentToWrite := createContentWithInfo("blob", fileContent)
	fileSHA, err := calculateSHA(contentToWrite)
	if err != nil {
		ePrintf("error in calculating the SHA: %s", err)
		os.Exit(1)
	}
	nFile, err := createEmptyObjectFile(fileSHA)
	if err != nil {
		ePrintf("error in creating the object file: %s", err)
		os.Exit(1)
	}
	err = createObjectFile(nFile, bytes.NewReader(contentToWrite))
	if err != nil {
		ePrintf("error in writing to the object file: %s", err)
		os.Exit(1)
	}
	fmt.Printf("%s\n", fileSHA)
}

func lsTreeCmd() {
	if len(os.Args) != 4 {
		ePrintf("usage: mygit ls-tree <flag> <file>\n")
		os.Exit(1)
	}
	if os.Args[2] != "--name-only" {
		ePrintf("usage: mygit cat-file --name-only <tree_sha>\n")
		os.Exit(1)
	}
	objHash := os.Args[3]
	if len(objHash) != 40 {
		ePrintf("invalid object hash: %q", objHash)
		os.Exit(1)
	}
	file, err := os.Open(fmt.Sprintf(".git/objects/%s/%s", objHash[0:2], objHash[2:]))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			ePrintf("no such object: %q", objHash)
			os.Exit(1)
		}
		ePrintf("could not open the object file: %v", err)
		os.Exit(1)
	}
	defer file.Close()
	content, objectType, err := readObjectFile(file)
	if err != nil {
		ePrintf("error in reading the object file: %s", err)
		os.Exit(1)
	}
	if objectType != "tree" {
		ePrintf("fatal: not a tree object: %q", objectType)
	}
	tree, err := readATreeObject(content)
	if err != nil {
		ePrintf("error in reading the tree object: %s", err)
		os.Exit(1)
	}
	for i := range tree {
		fmt.Println(tree[i].Name)
	}
}
