package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/codecrafters-io/git-starter-go/cmd/common"
)

const (
	defaultName    = "TestUser"
	defaultEmailID = "testuser@example.com"
)

type GitTree struct {
	Mode os.FileMode
	// GitMode is the stringification of the Mode by git standard
	// as the go stringfication and git stringication are different
	GitMode string
	Name    string
	// SHA is the actual SHA of the file without the hex encoding
	SHA [20]byte
}

type GitTrees []GitTree

// WriteTo will write the tree according to the git format
// it will also sort the entries by name
func (t GitTrees) WriteTo(w io.Writer) (int64, error) {
	// Sort entries lexicographically by name
	sort.Slice(t, func(i, j int) bool {
		return t[i].Name < t[j].Name
	})
	var n int64
	for _, entry := range t {
		n1, err := fmt.Fprintf(w, "%s %s", entry.GitMode, entry.Name)
		if err != nil {
			return n, err
		}
		n += int64(n1)
		n2, err := w.Write([]byte{0})
		if err != nil {
			return n, err
		}
		n += int64(n2)
		n3, err := w.Write(entry.SHA[:])
		if err != nil {
			return n, err
		}
		n += int64(n3)
	}
	return n, nil
}

// ParseTreeObjectBody unmarshal the byte array into GitTree object
// it is expected that the header would already been stripped from the content
// and we are indeed only getting the body of the tree object
func ParseTreeObjectBody(content []byte) ([]GitTree, error) {
	// a tree object is of the form
	//// tree <size>\0
	//// <mode> <name>\0<20_byte_sha>
	//// <mode> <name>\0<20_byte_sha>
	result, i := []GitTree{}, 0

	for i < len(content) {
		// Parse mode
		modeStart := i
		for content[i] != ' ' {
			i++
		}
		modeStr := string(content[modeStart:i])
		mode := modeFromGit(modeStr)
		i++ // Skip the space

		// Parse name
		nameStart := i
		for content[i] != 0 {
			i++
		}
		name := string(content[nameStart:i])
		i++ // Skip the null terminator

		// Parse SHA (20 bytes)
		if i+20 > len(content) {
			return nil, fmt.Errorf("unexpected end of content while reading SHA")
		}
		var sha [20]byte
		copy(sha[:], content[i:i+20])
		i += 20

		result = append(result, GitTree{
			Mode:    mode,
			GitMode: modeStr,
			Name:    name,
			SHA:     sha,
		})
	}

	return result, nil
}

// WriteTree generates a Git-like tree object for the specified directory and its contents.
//
// It recursively traverses the directory structure starting from `dirPath`, processing
// files and subdirectories to create entries for a Git tree object. The function serializes
// the tree into the Git object format and returns the SHA-1 hash of the tree object.
//
// Files and directories are processed as follows:
// - Files are read and their SHA-1 hashes are calculated based on their content.
// - Directories (other than `.git`) are recursively processed into sub-tree objects.
// - The `.git` directory is ignored during traversal.
//
// The function returns a 20-byte SHA-1 hash of the resulting tree object and an error if
// any issues occur during processing.
//
// Example:
//
//	sha, err := WriteTree("/path/to/repo")
//	if err != nil {
//		log.Fatalf("failed to write tree: %v", err)
//	}
//	fmt.Printf("Tree SHA: %x\n", sha)
func WriteTree(dirPath string) ([20]byte, error) {
	var buffer bytes.Buffer
	entries := []GitTree{}

	err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing %s: %w", path, err)
		}

		// Ignore the .git directory
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}

		if d.IsDir() {
			if path == dirPath {
				return nil
			}
			// Process subdirectories
			subTreeSHA, err := WriteTree(path)
			if err != nil {
				return err
			}
			entries = append(entries, GitTree{
				Mode:    d.Type(),
				GitMode: "40000",
				Name:    d.Name(),
				SHA:     subTreeSHA,
			})

			return filepath.SkipDir
		}

		// Process files
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open file %s: %w", path, err)
		}
		defer file.Close()

		fileContent, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("read file %s: %w", path, err)
		}

		fullContent := common.FormatGitObjectContent("blob", fileContent)
		rawSHA, err := common.CalculateSHA(fullContent)
		if err != nil {
			return fmt.Errorf("calculate file SHA for %s: %w", path, err)
		}

		mode := "100644" // Default mode for regular files
		if d.Type().Perm()&0111 != 0 {
			mode = "100755" // Executable files
		}

		entries = append(entries, GitTree{
			Mode:    d.Type(),
			GitMode: mode,
			Name:    d.Name(),
			SHA:     rawSHA,
		})
		return nil
	})
	if err != nil {
		return [20]byte{}, err
	}

	// write the entries to buffer
	_, err = GitTrees(entries).WriteTo(&buffer)
	if err != nil {
		return [20]byte{}, err
	}

	return bufferToFile(&buffer)
}

func bufferToFile(buffer *bytes.Buffer) ([20]byte, error) {
	// Compute the tree's SHA and write it to the object directory
	treeContent := buffer.Bytes()
	treeRawSHA, err := common.CalculateSHA(common.FormatGitObjectContent("tree", treeContent))
	if err != nil {
		return [20]byte{}, err
	}
	treeSHA := hex.EncodeToString(treeRawSHA[:])
	treeFile, err := common.CreateEmptyObjectFile(".", treeSHA)
	if err != nil {
		// the tree has been created and return the sha
		if os.IsExist(err) {
			return treeRawSHA, nil
		}
		return [20]byte{}, fmt.Errorf("couldn't create tree object file: %w", err)
	}
	defer treeFile.Close()
	err = common.WriteCompactContent(treeFile, bytes.NewReader(common.FormatGitObjectContent("tree", treeContent)))
	if err != nil {
		return [20]byte{}, err
	}
	return treeRawSHA, nil
}

// WriteCommitContent writes the content in the expected commit object form
func WriteCommitContent(treeSHA, commitMsg string, parentSHA ...string) ([]byte, error) {
	var buffer bytes.Buffer
	_, err := buffer.WriteString(fmt.Sprintf("tree %s\n", treeSHA))
	if err != nil {
		return nil, fmt.Errorf("write tree: %w", err)
	}
	for i := range parentSHA {
		_, err = buffer.WriteString(fmt.Sprintf("parent %s\n", parentSHA[i]))
		if err != nil {
			return nil, fmt.Errorf("write parent: %w", err)
		}
	}
	now := time.Now()
	_, err = buffer.WriteString(getAuthorCommiterString("author", now))
	if err != nil {
		return nil, fmt.Errorf("write author: %w", err)
	}
	_, err = buffer.WriteString(getAuthorCommiterString("committer", now))
	if err != nil {
		return nil, fmt.Errorf("write committer: %w", err)
	}
	err = buffer.WriteByte('\n')
	if err != nil {
		return nil, fmt.Errorf("write new line: %w", err)
	}
	_, err = buffer.WriteString(commitMsg + "\n")
	if err != nil {
		return nil, fmt.Errorf("write commitMsg: %w", err)
	}
	return buffer.Bytes(), nil
}

func getAuthorCommiterString(role string, time time.Time) string {
	timeUnix := time.Unix()
	_, offset := time.Zone()
	offsetHours := offset / 3600
	offsetMinutes := (offset % 3600) / 60
	tzSign := "+"
	if offset < 0 {
		tzSign = "-"
	}
	return fmt.Sprintf(
		"%s %s <%s> %d %s%02d%02d\n",
		role,
		defaultName,
		defaultEmailID,
		timeUnix,
		tzSign,
		offsetHours,
		offsetMinutes,
	)
}

func GetTreeHashFromCommit(commitHash, gitDir string) (string, error) {
	objFile, err := common.GetFileFromHash(gitDir, commitHash)
	if err != nil {
		return "", fmt.Errorf("GetTreeHashFromCommit: get file from hash: %w", err)
	}
	content, objType, err := common.ReadObjectFile(objFile)
	if err != nil {
		return "", fmt.Errorf("GetTreeHashFromCommit: read object file: %w", err)
	}
	if objType != "commit" {
		return "", fmt.Errorf("GetTreeHashFromCommit: expected commit, got %s", objType)
	}
	// Commit object content is like:
	// tree <tree-hash>
	// parent <parent-hash>
	// author ...
	// committer ...
	// <blank line>
	// Commit message
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "tree ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "tree ")), nil
		}
	}
	return "", fmt.Errorf("tree hash not found in commit object")
}
