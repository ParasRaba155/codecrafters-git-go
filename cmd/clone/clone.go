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

// readVarInt reads a variable-length integer from a byte slice.
// It updates the `offset` pointer as it consumes bytes from the `data` slice.
// This format is used for encoding sizes in Git delta instructions.
func readVarInt(data []byte, deltaOffset int) (size int, offset int, err error) {
	result := 0
	shift := 0
	for {
		// Ensure we don't read beyond the end of the data slice.
		if deltaOffset >= len(data) {
			return 0, 0, fmt.Errorf("unexpected end of data while reading variable-length integer")
		}
		b := data[deltaOffset] // Get the current byte.
		deltaOffset++          // Move the offset to the next byte.

		// Accumulate the lower 7 bits of the byte into the result.
		// Each subsequent byte shifts its bits by an additional 7.
		result |= (int(b) & 0x7f) << shift

		// If the Most Significant Bit (MSB) is 0, this is the last byte of the varint.
		if (b & 0x80) == 0 {
			break // Exit the loop as the varint is fully read.
		}

		// If MSB is 1, there's another byte to read. Increase the shift for the next byte.
		shift += 7

		// Prevent potential overflow for extremely large (and unlikely) varints,
		// or malformed data that could lead to an infinite loop if MSB is always 1.
		if shift >= 63 { // Max bits for int is usually 63 for 64-bit systems.
			return 0, 0, fmt.Errorf("variable-length integer too large or malformed")
		}
	}
	return result, deltaOffset, nil
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

	resultSize, deltaOffset, err := readVarInt(deltaInstructions, deltaOffset)
	if err != nil {
		return nil, fmt.Errorf("failed to read result object size from delta: %w", err)
	}

	resultBuffer := make([]byte, 0, resultSize)

	for deltaOffset < len(deltaInstructions) {
		commandByte := deltaInstructions[deltaOffset]
		deltaOffset++

		if (commandByte & 0x80) == 0 { // MSB is 0: Add literal data
			length := int(commandByte & 0x7f)
			if length == 0 { // Special case for length encoded in subsequent bytes
				// This is a simplified handler. A full implementation would read a varint for
				// length here. For now, if you encounter this, it means the delta is more complex
				// than this simplified parser handles. Git uses a single byte for small literal
				// lengths (0-127). For lengths > 127, it encodes them
				// as a varint where the first byte is 0, and the actual length follows as a varint.
				// This would involve another call to readVarInt here.
				// For many common deltas, this case might not be hit, but it's important for full
				// compliance.
				return nil, fmt.Errorf(
					"unsupported literal data length encoding (command byte 0x00). A varint for length is expected here.",
				)
			}

			if deltaOffset+length > len(deltaInstructions) {
				return nil, fmt.Errorf(
					"delta instructions truncated: literal data length %d exceeds remaining delta bytes at offset %d",
					length,
					deltaOffset-1,
				)
			}

			resultBuffer = append(
				resultBuffer,
				deltaInstructions[deltaOffset:deltaOffset+length]...)
			deltaOffset += length

		} else { // MSB is 1: Copy from base command
			offset := 0
			size := 0x10000 // Default size if no size bits are set

			// Read bytes for the offset
			// Bits 0-3 of the command byte determine how many bytes contribute to the offset.
			// Each bit, if set, means the next byte in the delta instructions contributes to the
			// offset.
			// The bytes are read in little-endian order.
			if (commandByte & 0x01) != 0 { // Bit 0
				if deltaOffset >= len(deltaInstructions) {
					return nil, fmt.Errorf("delta instructions truncated while reading offset byte 1")
				}
				offset |= int(deltaInstructions[deltaOffset]) << 0
				deltaOffset++
			}
			if (commandByte & 0x02) != 0 { // Bit 1
				if deltaOffset >= len(deltaInstructions) {
					return nil, fmt.Errorf("delta instructions truncated while reading offset byte 2")
				}
				offset |= int(deltaInstructions[deltaOffset]) << 8
				deltaOffset++
			}
			if (commandByte & 0x04) != 0 { // Bit 2
				if deltaOffset >= len(deltaInstructions) {
					return nil, fmt.Errorf("delta instructions truncated while reading offset byte 3")
				}
				offset |= int(deltaInstructions[deltaOffset]) << 16
				deltaOffset++
			}
			if (commandByte & 0x08) != 0 { // Bit 3
				if deltaOffset >= len(deltaInstructions) {
					return nil, fmt.Errorf("delta instructions truncated while reading offset byte 4")
				}
				offset |= int(deltaInstructions[deltaOffset]) << 24
				deltaOffset++
			}

			// Read bytes for the size
			// Bits 4-6 of the command byte determine how many bytes contribute to the size.
			// If none are set, the size defaults to 0x10000.
			// The bytes are read in little-endian order.
			// IMPORTANT: If a size byte is read, it *initializes* `size`,
			// otherwise the default `0x10000` is used.
			sizeBytesRead := 0
			if (commandByte & 0x10) != 0 { // Bit 4
				if deltaOffset >= len(deltaInstructions) {
					return nil, fmt.Errorf("delta instructions truncated while reading size byte 1")
				}
				size = int(deltaInstructions[deltaOffset]) << 0 // Initialize size with this byte
				deltaOffset++
				sizeBytesRead++
			}
			if (commandByte & 0x20) != 0 { // Bit 5
				if deltaOffset >= len(deltaInstructions) {
					return nil, fmt.Errorf("delta instructions truncated while reading size byte 2")
				}
				if sizeBytesRead == 0 { // If this is the first size byte read, initialize size
					size = int(deltaInstructions[deltaOffset]) << 8
				} else { // Otherwise, OR with the shifted byte
					size |= int(deltaInstructions[deltaOffset]) << 8
				}
				deltaOffset++
				sizeBytesRead++
			}
			if (commandByte & 0x40) != 0 { // Bit 6
				if deltaOffset >= len(deltaInstructions) {
					return nil, fmt.Errorf("delta instructions truncated while reading size byte 3")
				}
				if sizeBytesRead == 0 { // If this is the first size byte read, initialize size
					size = int(deltaInstructions[deltaOffset]) << 16
				} else { // Otherwise, OR with the shifted byte
					size |= int(deltaInstructions[deltaOffset]) << 16
				}
				deltaOffset++
				sizeBytesRead++
			}

			// Validate that the copy operation is within the bounds of the base content.
			if offset < 0 || size < 0 || offset+size > len(baseContent) {
				return nil, fmt.Errorf("copy command out of bounds: offset %d, size %d, base content length %d", offset, size, len(baseContent))
			}

			resultBuffer = append(resultBuffer, baseContent[offset:offset+size]...)
		}
	}

	if len(resultBuffer) != resultSize {
		return nil, fmt.Errorf(
			"resolved content size mismatch: expected %d bytes, actual %d bytes",
			resultSize,
			len(resultBuffer),
		)
	}

	return resultBuffer, nil
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
