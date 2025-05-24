package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/codecrafters-io/git-starter-go/cmd/clone"
	"github.com/codecrafters-io/git-starter-go/cmd/common"
)

// initCMD has the logic for the init subcommand
func initCMD() error {
	for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating directory: %w", err)
		}
	}

	headFileContents := []byte("ref: refs/heads/main\n")
	if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	fmt.Println("Initialized git directory")
	return nil
}

// catFileCmd has the logic for the cat-file subcommand
func catFileCmd(hash string) error {
	file, err := GetFileFromHash(hash)
	if err != nil {
		return fmt.Errorf("cat File command: get file from hash: %w", err)
	}
	defer file.Close()
	content, objectType, err := ReadObjectFile(file)
	if err != nil {
		return fmt.Errorf("error in reading the object file: %s", err)
	}
	if objectType != "blob" {
		return fmt.Errorf("the given hash object is not of type \"blob\" is %q", objectType)
	}
	fmt.Printf("%s", content)
	return nil
}

// hashObjectCmd has the logic for the hash-object subcommand
func hashObjectCmd(fileName string) error {
	file, err := os.Open(fileName)
	if err != nil {
		return fmt.Errorf("error in opening the given file: %w", err)
	}
	defer file.Close()
	fileContent, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("error in reading the given file: %w", err)
	}
	contentToWrite := FormatGitObjectContent("blob", fileContent)
	fileSHA, err := CalculateSHA(contentToWrite)
	if err != nil {
		return fmt.Errorf("error in calculating the SHA: %w", err)
	}
	nFile, err := CreateEmptyObjectFile(fileSHA)
	if err != nil {
		return fmt.Errorf("error in creating the object file: %w", err)
	}
	err = WriteCompactContent(nFile, bytes.NewReader(contentToWrite))
	if err != nil {
		return fmt.Errorf("error in writing to the object file: %w", err)
	}
	fmt.Printf("%s\n", fileSHA)
	return nil
}

func lsTreeCmd(hash string) error {
	file, err := GetFileFromHash(hash)
	if err != nil {
		return fmt.Errorf("ls tree command: get file from hash: %w", err)
	}
	defer file.Close()
	content, objectType, err := ReadObjectFile(file)
	if err != nil {
		return fmt.Errorf("error in reading the object file: %w", err)
	}
	if objectType != "tree" {
		return fmt.Errorf("fatal: not a tree object: %q", objectType)
	}
	tree, err := ParseTreeObjectBody(content)
	if err != nil {
		return fmt.Errorf("error in reading the tree object: %w", err)
	}
	for i := range tree {
		fmt.Println(tree[i].Name)
	}
	return nil
}

func writeTreeCmd() error {
	treeSHA, err := WriteTree(".")
	if err != nil {
		return fmt.Errorf("error in writing tree: %w", err)
	}
	fmt.Println(hex.EncodeToString(treeSHA[:]))
	return nil
}

func commitTreeCmd(treeSHA, commitSHA, commitMsg string) error {
	if len(treeSHA) != 40 {
		return fmt.Errorf("invalid treeSHA")
	}
	if len(commitSHA) != 40 {
		return fmt.Errorf("invalid commitSHA")
	}
	content, err := WriteCommitContent(treeSHA, commitMsg, commitSHA)
	if err != nil {
		return fmt.Errorf("write commit file: %w", err)
	}
	fullContent := FormatGitObjectContent("commit", content)
	fullContentSHA, err := CalculateSHA(fullContent)
	if err != nil {
		return fmt.Errorf("calculate full content sha: %w", err)
	}
	file, err := CreateEmptyObjectFile(fullContentSHA)
	if err != nil {
		return fmt.Errorf("create empty object file: %w", err)
	}
	err = WriteCompactContent(file, bytes.NewReader(fullContent))
	if err != nil {
		return fmt.Errorf("write object file: %s", err)
	}
	fmt.Printf("%s", fullContentSHA)
	return nil
}

func cloneCmd(repoLink, dirToCloneAt string) error {
	err := os.MkdirAll(dirToCloneAt, 0755) // 2147483648

	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("create the dir to clone the repo: %w", err)
	}
	err = os.Chdir(dirToCloneAt)
	if err != nil {
		return fmt.Errorf("couldn't change the dir: %w", err)
	}
	err = initCMD()
	if err != nil {
		return fmt.Errorf("couldn't initialize git: %w", err)
	}

	gitRefResponse, err := clone.GitSmartProtocolGetRefs(repoLink)
	if err != nil {
		return fmt.Errorf("git smart protocol for ref fetching: %w", err)
	}

	refs, err := clone.GetRefList(gitRefResponse)
	if err != nil {
		return fmt.Errorf("git smart protocol for ref list parsing: %w", err)
	}
	packfileContent, err := clone.RefDiscovery(repoLink, refs)
	if err != nil {
		return fmt.Errorf("git smart protocol for ref discovery: %w", err)
	}
	objects, err := clone.ReadPackFile(packfileContent)
	if err != nil {
		return err
	}
	err = clone.WriteObjects(dirToCloneAt, objects)
	if err != nil {
		return err
	}
	headIdx := slices.IndexFunc(refs, func(ref clone.GitRef) bool {
		return ref.Name == "HEAD"
	})
	if headIdx == -1 {
		return fmt.Errorf("head index not found")
	}
	headRef := refs[headIdx]
	treeSHA, err := GetTreeHashFromCommit(headRef.Hash, ".")
	if err != nil {
		return err
	}
	err = renderTree(treeSHA, ".", ".")
	if err != nil {
		return err
	}
	return nil
}

func renderTree(hash, workingDir, repoRoot string) error {
	objFile, err := common.GetFileFromHash(repoRoot, hash)
	if err != nil {
		return fmt.Errorf("renderTree: get file from hash: %w", err)
	}
	fileContent, objType, err := common.ReadObjectFile(objFile)
	if err != nil {
		return fmt.Errorf("renderTree: read the object file: %w", err)
	}
	if objType != "tree" {
		return fmt.Errorf("renderTree: got the object type %q for render Tree", objType)
	}
	treeEntry, err := ParseTreeObjectBody(fileContent)
	if err != nil {
		return fmt.Errorf("renderTree: could not parse tree: %w", err)
	}
	for _, entry := range treeEntry {
		entryPath := filepath.Join(workingDir, entry.Name)
		shaHex := hex.EncodeToString(entry.SHA[:])

		switch entry.GitMode {
		case "40000":
			err := os.MkdirAll(entryPath, 0755)
			if err != nil {
				return fmt.Errorf("renderTree: mkdir %s: %w", entryPath, err)
			}
			err = renderTree(shaHex, entryPath, repoRoot)
			if err != nil {
				return err
			}
		case "100644", "100755":
			objFile, err := common.GetFileFromHash(repoRoot, shaHex)
			if err != nil {
				return fmt.Errorf("renderTree: get file for blob %s: %w", shaHex, err)
			}
			content, objType, err := common.ReadObjectFile(objFile)
			if err != nil {
				return fmt.Errorf("renderTree: read blob file: %w", err)
			}
			if objType != "blob" {
				return fmt.Errorf("renderTree: expected blob, got %s", objType)
			}
			err = os.WriteFile(entryPath, content, entry.Mode)
			if err != nil {
				return fmt.Errorf("renderTree: writing blob to file %s: %w", entryPath, err)
			}
		default:
			return fmt.Errorf("renderTree: unsupported Git mode %q for entry %q", entry.GitMode, entry.Name)
		}
	}
	return nil
}
