package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
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

// readObjectFile will return the content after the null character byte
// and the type of the content e.g. the "tree", "blog", etc.
func readObjectFile(r io.Reader) ([]byte, string, error) {
	z, err := zlib.NewReader(r)
	if err != nil {
		return nil, "", err
	}
	defer z.Close()
	content, err := io.ReadAll(z)
	if err != nil {
		return nil, "", err
	}
	zeroPos := 0
	for _, by := range content {
		if by == 0 {
			break
		}
		zeroPos++
	}
	parts := bytes.Split(content[:zeroPos], []byte{' '})
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("couldn't find the object type")
	}
	return content[zeroPos+1:], string(parts[0]), nil
}

// createObjectFile writes the content byte to w with zlib compression
func createObjectFile(w io.Writer, content io.Reader) error {
	z := zlib.NewWriter(w)
	defer z.Close()

	contentByte, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("createObjectFile file could not read the content: %s", err)
	}

	n, err := z.Write(contentByte)
	if err != nil {
		return fmt.Errorf("createObjectFile file could not write the content: %s", err)
	}
	if n != len(contentByte) {
		return fmt.Errorf(
			"createObjectFile content length and written bytes do not match %d and %d",
			len(contentByte),
			n,
		)
	}
	return nil
}

// calculateSHA will return the sha after hex encoding
func calculateSHA(content []byte) (string, error) {
	hash, err := getRawSHA(content)
	if err != nil {
		return "", err
	}
	sha := hex.EncodeToString(hash[:])
	return sha, nil
}

// getRawSHA for raw 20 bytes hash
func getRawSHA(content []byte) ([20]byte, error) {
	hasher := sha1.New()
	n, err := hasher.Write(content)
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
	res := hasher.Sum(nil)
	if len(res) != 20 {
		return [20]byte{}, fmt.Errorf("malformed hash created with '%d' bytes", len(res))
	}
	return [20]byte(res), nil
}

// createEmptyObjectFile will crete sha[0:2],sha[2:40]
func createEmptyObjectFile(sha string) (*os.File, error) {
	if len(sha) != 40 {
		return nil, fmt.Errorf("invalid length of sha object: %d", len(sha))
	}
	dir, rest := sha[0:2], sha[2:]
	err := os.Mkdir(fmt.Sprintf("./.git/objects/%s", dir), fs.FileMode(os.ModeDir))
	if err != nil && !os.IsExist(err) {
		return nil, err
	}
	return os.Create(fmt.Sprintf("./.git/objects/%s/%s", dir, rest))
}

// createContentWithInfo
func createContentWithInfo(typ string, content []byte) []byte {
	contentLength := len(content)
	contentDigitLength := numOfDigits(contentLength)

	result := make([]byte, 0, len(typ)+contentLength+1+contentDigitLength+len(content))
	// append type
	result = append(result, typ...)
	// append the space
	result = append(result, ' ')
	// append the size
	result = append(result, []byte(fmt.Sprintf("%d", contentLength))...)
	// append the null byte
	result = append(result, 0)
	// append the content
	result = append(result, content...)
	return result
}

func numOfDigits(a int) int {
	count := 0
	for a != 0 {
		a /= 10
		count++
	}
	return count
}

// readATreeObject unmarshal the byte array into GitTree object
// it is expected that the header would already been stripped from the content
// and we are indeed only getting the body of the tree object
func readATreeObject(content []byte) ([]GitTree, error) {
	// a tree object is of the form
	//// tree <size>\0
	//// <mode> <name>\0<20_byte_sha>
	//// <mode> <name>\0<20_byte_sha>
	result := []GitTree{}

	beforeSpace := 0
	beforeName := 0
	for i := 0; i < len(content); i++ {
		curr := GitTree{}
		if content[i] == ' ' {
			fileMode := content[beforeSpace:i]
			mode, err := strconv.Atoi(string(fileMode))
			if err != nil {
				return nil, err
			}
			curr.Mode = fs.FileMode(mode)
			beforeName = i + 1
		}
		if content[i] == 0 {
			// Extract name
			name := content[beforeName:i]

			// Ensure there are at least 20 bytes for the SHA
			if i+1+20 > len(content) {
				return nil, fmt.Errorf("unexpected end of content while reading SHA")
			}

			// Extract and copy the SHA
			var sha [20]byte
			copy(sha[:], content[i+1:i+1+20])

			curr.Name = string(name)
			curr.SHA = sha

			// Move to the next entry
			beforeSpace = i + 21
			i += 20 // Skip over the SHA bytes
			result = append(result, curr)
		}
	}
	return result, nil
}

func writeTree(dirPath string) ([20]byte, error) {
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
			subTreeSHA, err := writeTree(path)
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

		fullContent := createContentWithInfo("blob", fileContent)
		rawSHA, err := getRawSHA(fullContent)
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
	treeRawSHA, err := getRawSHA(createContentWithInfo("tree", treeContent))
	if err != nil {
		return [20]byte{}, err
	}
	treeSHA := hex.EncodeToString(treeRawSHA[:])
	treeFile, err := createEmptyObjectFile(treeSHA)
	if err != nil {
		// the tree has been created and return the sha
		if os.IsExist(err) {
			return treeRawSHA, nil
		}
		return [20]byte{}, fmt.Errorf("couldn't create tree object file: %w", err)
	}
	defer treeFile.Close()
	err = createObjectFile(treeFile, bytes.NewReader(createContentWithInfo("tree", treeContent)))
	if err != nil {
		return [20]byte{}, err
	}
	return treeRawSHA, nil
}

func writeCommitContent(treeSHA, commitMsg string, parentSHA ...string) ([]byte, error) {
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
