package clone

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
)

const gitUploadPack = "git-upload-pack"

type GitRef struct {
	Hash string
	Name string
}

type PackHeader struct {
	Version      uint32
	NumOfObjects uint32
}

func GitSmartProtocolGetRefs(repLink string) ([]byte, error) {
	fmt.Printf("DEBUG: repo link provide: %s\n", repLink)
	refUrl := fmt.Sprintf("%s/info/refs?service=%s", repLink, gitUploadPack)
	gitResponse, err := http.Get(refUrl)
	if err != nil {
		return nil, fmt.Errorf("get refs via smart protocol: %w", err)
	}
	if gitResponse.StatusCode != 200 {
		return nil, fmt.Errorf(
			"get refs via smart protocol: invalid status code %d %s",
			gitResponse.StatusCode,
			gitResponse.Status,
		)
	}
	defer gitResponse.Body.Close()
	content, err := io.ReadAll(gitResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("get refs via smart protocol: read response: %w", err)
	}
	return content, nil
}

func GetRefList(input []byte) ([]GitRef, error) {
	refParts := bytes.Split(input, []byte{'\n'})
	if len(refParts) < 2 {
		return nil, fmt.Errorf("invalid length for ref list")
	}

	refList := make([]GitRef, 0, len(refParts)-2)
	for lineNum, line := range refParts[1:] {
		if bytes.Equal(line, []byte{'0', '0', '0', '0'}) {
			break
		}
		// on 2nd line the first 4 bytes are "0000" we can ignore those
		if lineNum == 0 {
			line = line[4:]
		}
		// ignore the 4 size bytes
		line = line[4:]
		hashBytes := line[:40]
		line = line[40:]
		if line[0] != ' ' {
			panic("FUCK we should have got a space")
		}
		line := line[1:]
		lineParts := bytes.Split(line, []byte{0}) // split by null byte
		nameBytes := lineParts[0]
		refList = append(refList, GitRef{
			Hash: string(hashBytes),
			Name: string(nameBytes),
		})
	}
	return refList, nil
}

func RefDiscovery(repoLink string, refs []GitRef) ([]byte, error) {
	fullURL := fmt.Sprintf("%s/git-upload-pack", repoLink)
	request, err := http.NewRequest("POST", fullURL, bytes.NewReader(generateRefDiscoveryRequest(refs)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-git-upload-pack-request")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("RefDiscovery Client Do: %w", err)
	}
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("RefDiscovery client response invalid status code: %s", response.Status)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("RefDiscovery read response: %w", err)
	}
	return content, nil
}

func generateRefDiscoveryRequest(refs []GitRef) []byte {
	// request is of the format
	// 0032want <40-char-ref>\n
	// 0032want <40-char-ref>\n
	// ....
	// 00000009done\n
	capacity := 50*len(refs) + 4 + 9
	request := make([]byte, 0, capacity)
	for i := range refs {
		current := fmt.Sprintf("0032want %s\n", refs[i].Hash)
		request = append(request, []byte(current)...)
	}
	request = append(request, []byte("00000009done\n")...)
	return request
}

func ReadPackFile(content []byte) error {
	fmt.Printf("ReadPackFile content length: %d\n", len(content))
	offset, packHeader, err := readPackFileHeader(content)
	if err != nil {
		return fmt.Errorf("ReadPackFile: read header: %w", err)
	}
	content = content[offset:]
	err = readPackFileBody(content, int(packHeader.NumOfObjects))
	if err != nil {
		return fmt.Errorf("err: %w", err)
	}
	return nil
}

// readPackFileHeader will read the header, and return the number of bytes
// read by it (offset) along side the header and error
func readPackFileHeader(content []byte) (int, PackHeader, error) {
	offset, packHeader := 0, PackHeader{}
	if bytes.Equal(content[offset:offset+8], []byte{'0', '0', '0', '8', 'N', 'A', 'K', '\n'}) {
		// return offset, packHeader, fmt.Errorf("first 8 bytes must be 0008NAK: %s", content[offset:offset+8])
		offset += 8
	}

	if !bytes.Equal(content[offset:offset+4], []byte{'P', 'A', 'C', 'K'}) {
		return offset, packHeader, fmt.Errorf("first 4 bytes must be PACK: %s", content[offset:offset+4])
	}
	offset += 4

	version := readBigEndian([4]byte(content[offset : offset+4]))
	if version != 2 && version != 3 {
		return offset, packHeader, fmt.Errorf("invalid pack file version: %d", version)
	}
	offset += 4

	numOfObject := readBigEndian([4]byte(content[offset : offset+4]))
	offset += 4
	packHeader.NumOfObjects = numOfObject
	packHeader.Version = version
	return offset, packHeader, nil
}

func readPackFileBody(content []byte, numOfObj int) error {
	offset := 0
	fmt.Printf("readPackFileBody = %d\n", len(content))
	for i := range numOfObj {
		length, objType, headerBytesRead, err := packObjectSize(content[offset:])
		if err != nil {
			return fmt.Errorf("reading the size of %d object: %w", i, err)
		}

		switch objType {
		case OBJ_TAG, OBJ_BLOB, OBJ_COMMIT, OBJ_TREE:
		case OBJ_REF_DELTA:
			basObjHash := hex.EncodeToString(content[offset : offset+20])
			offset += 20
			fmt.Printf("basObjHash: %s\n", basObjHash)
		case OBJ_OFS_DELTA:
		default:
			panic(fmt.Sprintf("unimplemented %s", objType))
		}

		compressed, decompressed, used, err := findAndDecompress(content[offset+headerBytesRead:])
		if err != nil {
			return fmt.Errorf("decompressing object %d: %w", i, err)
		}

		fmt.Printf(
			"%03d object type = %s, declared length = %d, headerBytesRead  = %d, compressed bytes = %d, decompressed bytes = %d used = %d\n",
			i, objType, length, headerBytesRead, len(compressed), len(decompressed), used,
		)

		offset += headerBytesRead + used
		fmt.Printf("offset: %d\n", offset)
		if offset > len(content) {
			return fmt.Errorf("offset %d exceeded content length %d after object %d", offset, len(content), i)
		}
	}
	fmt.Printf("final body offset: %d\n", offset)
	return nil
}
