package clone

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"

	"github.com/codecrafters-io/git-starter-go/cmd/common"
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
	request, err := http.NewRequest(
		"POST",
		fullURL,
		bytes.NewReader(generateRefDiscoveryRequest(refs)),
	)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-git-upload-pack-request")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("RefDiscovery Client Do: %w", err)
	}
	if response.StatusCode != 200 {
		return nil, fmt.Errorf(
			"RefDiscovery client response invalid status code: %s",
			response.Status,
		)
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

func ReadPackFile(content []byte) ([]GitObject, error) {
	offset, packHeader, err := readPackFileHeader(content)
	if err != nil {
		return nil, fmt.Errorf("ReadPackFile: read header: %w", err)
	}
	content = content[offset:]
	objects, err := readPackFileBody(content, int(packHeader.NumOfObjects))
	if err != nil {
		return nil, fmt.Errorf("ReadPackFile: read body: %w", err)
	}
	return objects, nil
}

// readPackFileHeader will read the header, and return the number of bytes
// read by it (offset) along side the header and error
func readPackFileHeader(content []byte) (int, PackHeader, error) {
	offset, packHeader := 0, PackHeader{}
	if bytes.Equal(content[offset:offset+8], []byte{'0', '0', '0', '8', 'N', 'A', 'K', '\n'}) {
		offset += 8
	}

	if !bytes.Equal(content[offset:offset+4], []byte{'P', 'A', 'C', 'K'}) {
		return offset, packHeader, fmt.Errorf(
			"first 4 bytes must be PACK: %s",
			content[offset:offset+4],
		)
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

func readPackFileBody(content []byte, numOfObj int) ([]GitObject, error) {
	offset := 0
	objects := make([]GitObject, numOfObj)
	for i := range numOfObj {
		currentObj := GitObject{}
		_, objType, headerBytesRead, err := packObjectSize(content[offset:])
		if err != nil {
			return nil, fmt.Errorf("reading the size of %d object: %w", i, err)
		}
		offset += headerBytesRead

		switch objType {
		case OBJ_TAG, OBJ_BLOB, OBJ_COMMIT, OBJ_TREE:
		case OBJ_REF_DELTA:
			basObjHash := hex.EncodeToString(content[offset : offset+20])
			offset += 20
			currentObj.Base = basObjHash
		case OBJ_OFS_DELTA:
		default:
			panic(fmt.Sprintf("unimplemented %s", objType))
		}

		_, decompressed, used, err := findAndDecompress(content[offset:])
		if err != nil {
			return nil, fmt.Errorf("decompressing object %d: %w", i, err)
		}

		currentObj.ObjectType, currentObj.Size, currentObj.Content = objType, len(
			decompressed,
		), decompressed
		objects[i] = currentObj

		offset += used
		if offset > len(content) {
			return nil, fmt.Errorf(
				"offset %d exceeded content length %d after object %d",
				offset,
				len(content),
				i,
			)
		}
	}
	return objects, nil
}

// readVarInt is reading the size of data in the same way as we did in `packObjectSize`. The
// difference here is that in `packObjectSize` first byte also contains the information about object
// type whereas here we directly read the 7 bytes as size bytes and MSB as way to continue or not
func readVarInt(data []byte, offset int) (size int, newOffset int, err error) {
	result, shift := 0, 0
	for {
		if offset >= len(data) {
			return 0, 0, fmt.Errorf("unexpected end of data while reading variable-length integer")
		}
		b := data[offset]
		offset++

		// 0x7f -> 0b01111111
		// 0x80 -> 0b10000000
		result |= (int(b) & 0x7f) << shift
		if (b & 0x80) == 0 {
			break
		}

		shift += 7

		if shift >= 63 {
			return 0, 0, fmt.Errorf("variable-length integer too large or malformed")
		}
	}
	return result, offset, nil
}

// Applies the delta to a base object and returns the final object bytes.
func applyDelta(baseContent, deltaInstructions []byte) ([]byte, error) {
	deltaOffset := 0

	baseSizeFromDelta, deltaOffset, err := readVarInt(deltaInstructions, deltaOffset)
	if err != nil {
		return nil, fmt.Errorf("failed to read base object size from delta: %w", err)
	}

	if baseSizeFromDelta != len(baseContent) {
		return nil, fmt.Errorf(
			"base object size mismatch: delta expects %d bytes, actual base is %d bytes",
			baseSizeFromDelta,
			len(baseContent),
		)
	}

	size, deltaOffset, err := readVarInt(deltaInstructions, deltaOffset)
	if err != nil {
		return nil, fmt.Errorf("failed to read result object size from delta: %w", err)
	}

	result := make([]byte, 0, size)

	for deltaOffset < len(deltaInstructions) {
		commandByte := deltaInstructions[deltaOffset]
		deltaOffset++

		// 0x80 -> 0b10000000
		// 0x7f -> 0b01111111
		// 0x01 -> 0b00000001
		// 0x02 -> 0b00000010
		// 0x04 -> 0b00000100
		// 0x08 -> 0b00001000
		// 0x10 -> 0b00010000
		// 0x20 -> 0b00100000
		// 0x40 -> 0b01000000
		// 0x10000 -> 0b00010000000000000000 (65536)

		// MSB is 0: Add literal data
		if (commandByte & 0x80) == 0 {
			length := int(commandByte & 0x7f)
			if length == 0 {
				// Special case for length encoded in subsequent bytes
				// This is a simplified handler. A full implementation would read a varint for
				// length here. For now, if we encounter this, it means the delta is more complex
				// than this simplified parser handles. Git uses a single byte for small literal
				// lengths (0-127). For lengths > 127, it encodes them
				// as a varint where the first byte is 0, and the actual length follows as a varint.
				// This would involve another call to readVarInt here.
				// For many common deltas, this case might not be hit, but it's important for full
				// compliance.
				return nil, fmt.Errorf(
					"unsupported literal data length encoding (command byte 0x00). A varint for length is expected here",
				)
			}

			if deltaOffset+length > len(deltaInstructions) {
				return nil, fmt.Errorf(
					"delta instructions truncated: literal data length %d exceeds remaining delta bytes at offset %d",
					length,
					deltaOffset-1,
				)
			}

			result = append(
				result,
				deltaInstructions[deltaOffset:deltaOffset+length]...,
			)
			deltaOffset += length
			continue
		}
		// MSB is 1: Copy from base command
		offset := 0
		size := 0x10000 // Default size if no size bits are set

		// Read bytes for the offset
		// Bits 0-3 of the command byte determine how many bytes contribute to the offset.
		// Each bit, if set, means the next byte in the delta instructions contributes to the
		// offset.
		// The bytes are read in little-endian order.
		lowerBits := [...]byte{0x01, 0x02, 0x04, 0x08}
		for i, bit := range lowerBits {
			if (commandByte & bit) == 0 {
				continue
			}
			if deltaOffset >= len(deltaInstructions) {
				return nil, fmt.Errorf("delta instructions truncated while reading offset byte %d", i+1)
			}
			offset |= int(deltaInstructions[deltaOffset]) << (8 * i)
			deltaOffset++
		}

		// Read bytes for the size
		// Bits 4-6 of the command byte determine how many bytes contribute to the size.
		// If none are set, the size defaults to 0x10000.
		// The bytes are read in little-endian order.
		// IMPORTANT: If a size byte is read, it *initializes* `size`,
		// otherwise the default `0x10000` is used.
		sizeBytesRead := 0
		higherBits := [...]byte{0x10, 0x20, 0x40}
		for i, bit := range higherBits {
			if (commandByte & bit) == 0 {
				continue
			}
			if deltaOffset >= len(deltaInstructions) {
				return nil, fmt.Errorf("delta instructions truncated while reading size byte %d", i+1)
			}
			// if it's the first byte read then initialize size
			if i == 0 || sizeBytesRead == 0 {
				size = int(deltaInstructions[deltaOffset]) << (8 * i)
				deltaOffset++
				sizeBytesRead++
				continue
			}
			// Otherwise OR it
			size |= int(deltaInstructions[deltaOffset]) << (8 * i)

			deltaOffset++
			sizeBytesRead++
		}

		// Validate that the copy operation is within the bounds of the base content.
		if offset < 0 || size < 0 || offset+size > len(baseContent) {
			return nil, fmt.Errorf("copy command out of bounds: offset %d, size %d, base content length %d", offset, size, len(baseContent))
		}

		result = append(result, baseContent[offset:offset+size]...)
	}

	if len(result) != size {
		return nil, fmt.Errorf(
			"resolved content size mismatch: expected %d bytes, actual %d bytes",
			size,
			len(result),
		)
	}

	return result, nil
}

func WriteObjects(dir string, objects []GitObject) error {
	var deltas []GitObject

	for i, obj := range objects {
		if obj.ObjectType == OBJ_REF_DELTA {
			deltas = append(deltas, obj)
			continue
		}

		// First pass: Write non-delta objects
		if err := writeSingleObject(dir, obj); err != nil {
			return fmt.Errorf("WriteObjects pass1 [%d]: %w", i, err)
		}
	}

	for i, obj := range deltas {
		// Second pass: Resolve and write REF_DELTA objects
		if err := writeDeltaObject(dir, obj); err != nil {
			return fmt.Errorf("WriteObjects pass2 delta [%d]:%+v %w", i, obj, err)
		}
	}

	return nil
}

func writeSingleObject(dir string, obj GitObject) error {
	fullContent := common.FormatGitObjectContent(obj.ObjectType.String(), obj.Content)
	hash, err := common.CalculateEncodedSHA(fullContent)
	if err != nil {
		return fmt.Errorf("calculate SHA: %w", err)
	}
	file, err := common.CreateEmptyObjectFile("", hash)
	if err != nil {
		return fmt.Errorf("create object file: %w", err)
	}
	return common.WriteCompactContent(file, bytes.NewReader(fullContent))
}

func writeDeltaObject(dir string, obj GitObject) error {
	file, err := common.GetFileFromHash(".", obj.Base)
	if err != nil {
		return fmt.Errorf("get base object: %w", err)
	}

	baseContent, baseTypeStr, err := common.ReadObjectFile(file)
	if err != nil {
		return fmt.Errorf("read base object: %w", err)
	}

	resolvedContent, err := applyDelta(baseContent, obj.Content)
	if err != nil {
		return fmt.Errorf("apply delta: %w", err)
	}

	objType := StringToObjectType(baseTypeStr)
	if objType == OBJ_INVALID {
		return fmt.Errorf("invalid base type: %s", baseTypeStr)
	}

	resolvedObj := GitObject{
		ObjectType: objType,
		Content:    resolvedContent,
	}

	return writeSingleObject(dir, resolvedObj)
}
