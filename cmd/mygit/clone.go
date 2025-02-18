package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	errInvalidPacketLineLength = errors.New("invalid packet length")
	errNoWants                 = errors.New("no wants provided in the body")
	errInvalidObjectType       = errors.New("invalid object type")
)

var (
	objectIDRegex  = regexp.MustCompile(`[0-9a-f]{40}`)
	refRecordRegex = regexp.MustCompile(`([a-f0-9]{40})\srefs/(.*)`)
	ackHeader      = []byte("ACK")
	nakHeader      = []byte("NAK")
	pack           = []byte("PACK")
)

var client = http.Client{
	Timeout: 5 * time.Second,
}

const (
	service    = "service"
	uploadPack = "git-upload-pack"
	zeroID     = "0000000000000000000000000000000000000000"
	refName    = "HEAD"
)

type PacketLine struct {
	Content       []byte
	Size          int
	IsFlushPacket bool
}

func (l PacketLine) String() string {
	return fmt.Sprintf("{Content: %q, Size: %d, IsFlushPacket: %t}", l.Content, l.Size, l.IsFlushPacket)
}

// validateHeader: checks for given packet line to be a header
//
// the header is of the format: `4*(HEXDIGITS)# service=$servicename`
func (pktLine PacketLine) validateHeader() error {
	if want := fmt.Sprintf("# %s=%s", service, uploadPack); !bytes.Equal(pktLine.Content, []byte(want)) {
		return fmt.Errorf("packet header expectation failed, want: %q got: %q", want, pktLine.Content)
	}
	return nil
}

type RefRecord struct {
	ObjID string
	Name  string
}

type CommitRef struct {
	Head      string
	Signature string
	Version   int
}

// readPktLine returns the content after checking it's size, it return the size of the content
// It strips the first size 4 bytes of the result size, so the caller knows how much bytes it needs to read
// It checks for the special "0000" FLUSH-PACKET
func readPktLine(body io.Reader) (PacketLine, error) {
	var pktLine PacketLine
	lengthBuffer := [4]byte{}
	_, err := io.ReadFull(body, lengthBuffer[:])
	if err != nil {
		return pktLine, fmt.Errorf("read packet length: %w", err)
	}

	pktLength, err := strconv.ParseInt(string(lengthBuffer[:]), 16, 64)
	if err != nil {
		return pktLine, fmt.Errorf("read packet length: %w", err)
	}
	pktLine.Size = int(pktLength)

	if pktLength == 0 {
		pktLine.IsFlushPacket = true
		return pktLine, nil
	}

	if pktLength == 4 {
		return pktLine, fmt.Errorf("%w :packet size 4", errInvalidPacketLineLength)
	}

	contentBuffer := make([]byte, pktLength-4)
	_, err = io.ReadFull(body, contentBuffer)
	if err != nil {
		return pktLine, fmt.Errorf("read packet content: %w", err)
	}

	// ignore the line feed char, if it exists
	if contentBuffer[len(contentBuffer)-1] == '\n' {
		contentBuffer = contentBuffer[:len(contentBuffer)-1]
	}
	pktLine.Content = contentBuffer
	return pktLine, nil
}

// FetchRefs
func FetchRefs(repoURL string) (io.ReadCloser, error) {
	infoRefsAppendedURL, err := url.JoinPath(repoURL, "/info/refs")
	if err != nil {
		return nil, fmt.Errorf("join path: %w", err)
	}
	parsedURL, err := url.Parse(infoRefsAppendedURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	queries := url.Values{}
	queries.Add(service, uploadPack)
	parsedURL.RawQuery = queries.Encode()
	getResponse, err := client.Get(parsedURL.String())
	if err != nil {
		return nil, fmt.Errorf("URL = %q, failed to get:%w", parsedURL.String(), err)
	}
	responseContentType := getResponse.Header.Get("Content-Type")
	if responseContentType != fmt.Sprintf("application/x-%s-advertisement", uploadPack) {
		return nil, fmt.Errorf("not a smart protocol: %q content type URL = %q", responseContentType, parsedURL.String())
	}
	statusCode := getResponse.StatusCode
	if statusCode != http.StatusOK && statusCode != http.StatusNotModified {
		return nil, fmt.Errorf("URL = %q, status-code %d", parsedURL.String(), statusCode)
	}
	return getResponse.Body, nil
}

// ParsePacketFile validates the packet headers and body and returns the packet file format
func ParsePacketFile(body io.ReadCloser) ([]PacketLine, error) {
	defer func() {
		if err := body.Close(); err != nil {
			log.Printf("[ERROR] validatePacketFile close error: %v", err)
		}
	}()

	result := []PacketLine{}

	for {
		pktLine, err := readPktLine(body)
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		result = append(result, pktLine)
	}
	if err := result[0].validateHeader(); err != nil {
		return nil, fmt.Errorf("invalid packet header:%w", err)
	}

	return result, nil
}

// RefRecordsFromPacketLines returns the RefRecords reading from the packet lines
//
// TODO: add error handling
func RefRecordsFromPacketLines(pktLines []PacketLine) ([]RefRecord, error) {
	records := make([]RefRecord, 0, len(pktLines)-2)
	if len(pktLines) <= 2 {
		log.Printf("[WARN] insufficient number of packet lines: %d", len(pktLines))
	}
	// first ref is the default HEAD ref
	pktLine := pktLines[2]
	if pktLine.Size < 40 {
		log.Printf("[WARN] insufficient size of  head packet line: %d", pktLine.Size)
	}
	if !bytes.Contains(pktLine.Content, []byte(refName)) {
		log.Printf("[WARN] required ref name absent: %q", pktLine.Content)
	}
	hash := pktLine.Content[:40]
	if !objectIDRegex.Match(hash) {
		log.Printf("[WARN] invalid hash: %q", pktLine.Content)
	}
	headRef := RefRecord{
		ObjID: string(hash),
		Name:  refName,
	}
	records = append(records, headRef)
	for _, pktLine := range pktLines[3 : len(pktLines)-1] {
		matches := refRecordRegex.FindSubmatch(pktLine.Content)
		if len(matches) != 3 {
			log.Printf("failed for pkt line with content: %q", pktLine.Content)
			continue
		}
		records = append(records, RefRecord{
			ObjID: string(matches[1]),
			Name:  string(matches[2]),
		})
	}
	return records, nil
}

func DiscoverRef(repoURL string, refs []RefRecord) (io.ReadCloser, error) {
	url, err := url.JoinPath(repoURL, uploadPack)
	if err != nil {
		return nil, fmt.Errorf("join path: %w", err)
	}

	want := getAllWants(refs)
	contentType := fmt.Sprintf("application/x-%s-request", uploadPack)
	requestBody, err := createRefDiscoveryRequestBody(want, nil)
	if err != nil {
		return nil, err
	}
	postResponse, err := client.Post(url, contentType, requestBody)
	if err != nil {
		return nil, fmt.Errorf("URL = %q, failed to get:%w", url, err)
	}
	responseContentType := postResponse.Header.Get("Content-Type")
	if responseContentType != fmt.Sprintf("application/x-%s-result", uploadPack) {
		return nil, fmt.Errorf("ref discovery content is %q for %q", responseContentType, url)
	}
	statusCode := postResponse.StatusCode
	if statusCode != http.StatusOK && statusCode != http.StatusNotModified {
		return nil, fmt.Errorf("URL = %q, status-code %d", url, statusCode)
	}
	return postResponse.Body, nil
}

// createRefDiscoveryRequestBody creates the required body for ref discovery request
// if there are no wants it returns errNoWants error
func createRefDiscoveryRequestBody(want []string, have []string) (io.Reader, error) {
	if len(want) == 0 {
		return nil, errNoWants
	}
	// the body is of the format
	// 0032want <obj-id>\n
	// 0032have <obj-id>\n
	// 0000
	// The '0000' is flush packet, and we should be fine with hard-coding the '0032'
	// since the length of given packet line for both the want and have will always be 50
	// and 50 in hex is 0032
	var b strings.Builder
	for i := range want {
		b.WriteString("0032want ")
		b.WriteString(want[i])
		b.WriteByte('\n')
	}
	for i := range have {
		b.WriteString("0032have ")
		b.WriteString(have[i])
		b.WriteByte('\n')
	}
	b.WriteString("0000")
	b.WriteString("0009done\n")

	return strings.NewReader(b.String()), nil
}

func getAllWants(refs []RefRecord) []string {
	result := make([]string, len(refs))
	for i := range refs {
		result[i] = refs[i].ObjID
	}
	return result
}

func ParseDiscoverRefResponse(body io.ReadCloser) error {
	defer func() {
		if err := body.Close(); err != nil {
			log.Printf("[ERROR] parseRefDiscoveryResponse close error: %v", err)
		}
	}()
	// first we get a packet line with 0008ACK or 0008NAK
	pkt, err := readPktLine(body)
	if err != nil {
		return fmt.Errorf("reading ack packet: %w", err)
	}
	if !bytes.Equal(pkt.Content, ackHeader) && !bytes.Equal(pkt.Content, nakHeader) {
		return fmt.Errorf("packet is neither ACK not NAK: %v", pkt.Content)
	}
	objCount, err := validatePackFileHeader(body)
	if err != nil {
		return fmt.Errorf("invalid pack header: %w", err)
	}
	var i uint32
	for i = 0; i < objCount; i++ {
		objType, size, err := readPackObjectSize(body)
		if err != nil {
			return fmt.Errorf("read pack object size: %w", err)
		}
		fmt.Printf("[INFO] %03d reading %s with %d size\n", i, objType, size)
		temp1 := make([]byte, size)
		_, err = io.ReadFull(body, temp1)
		if err != nil {
			return fmt.Errorf("reading only %d content: %w", size, err)
		}
		_, err = ParsePacketObject(bytes.NewReader(temp1), objType)
		if err != nil {
			return fmt.Errorf("reading packet object: %w", err)
		}
	}
	return nil
}

// validatePackFileHeader valides the header of pack file
// and it returns the number of objects in the pack file
func validatePackFileHeader(body io.Reader) (uint32, error) {
	packBuf := [4]byte{}
	_, err := io.ReadFull(body, packBuf[:])
	if err != nil {
		return 0, fmt.Errorf("pack header: read PACK: %w", err)
	}
	if !bytes.Equal(packBuf[:], pack) {
		return 0, fmt.Errorf("pack header: did not get pack: %v", packBuf)
	}
	versionBuf := [4]byte{}
	_, err = io.ReadFull(body, versionBuf[:])
	if err != nil {
		return 0, fmt.Errorf("pack header: reading version: %w", err)
	}
	version := GetIntFromBigIndian(versionBuf)
	if version != 2 && version != 3 {
		return 0, fmt.Errorf("pack header: invalid version: %d", version)
	}
	numOfObjBuf := [4]byte{}
	_, err = io.ReadFull(body, numOfObjBuf[:])
	if err != nil {
		return 0, fmt.Errorf("pack header: reading number of object: %w", err)
	}
	return GetIntFromBigIndian(numOfObjBuf), nil
}

func readPackObjectSize(r io.Reader) (ObjectType, int, error) {
	buf := [1]byte{} // we will read the first byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, 0, fmt.Errorf("read first byte of pack object: %w", err)
	}
	// NOTE: 0x0F: `0b00001111` to extract the lower 4 bits
	// NOTE 0x07: `0b00000111` to extract the lower 3 bits
	// NOTE: 0x80: `0b10000000` to extract the MSB (Most Significant Bit)
	// NOTE: 0x7F: `0b01111111` to extract the last 7 bits

	b := buf[0]
	size := int(b & 0x0F)
	objType := (b >> 4) & 0x07

	shift := 4
	for (b & 0x80) != 0 { // While MSB is set
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, 0, fmt.Errorf("read size bytes: %w", err)
		}
		b = buf[0]
		size |= int(b&0x7F) << shift
		shift += 7
	}
	return validateObjectType(objType), size, nil
}

func ParsePacketObject(r io.Reader, objType ObjectType) ([]byte, error) {
	switch objType {
	case OBJ_INVALID:
		return nil, fmt.Errorf("read packet object %s :%w", OBJ_INVALID, errInvalidObjectType)
	case OBJ_COMMIT, OBJ_TREE, OBJ_BLOB, OBJ_TAG:
		return parseUndeltifiedPackObject(r, objType)
	case OBJ_OFS_DELTA:
		return parseOffsetDeltaObject(r)
	case OBJ_REF_DELTA:
		return parseRefDeltaObject(r)
	default:
		return nil, fmt.Errorf("read packet object %s :%w", objType, errInvalidObjectType)
	}
}

// parseUndeltifiedPackObject for parsing pack objects with types "commit", "tag", "blob", "tree"
func parseUndeltifiedPackObject(r io.Reader, typ ObjectType) ([]byte, error) {
	decompressedContent, err := ReadCompressed(r)
	if err != nil {
		return nil, fmt.Errorf("parse undeltified: read object: %w", err)
	}
	fmt.Println("---------------------------------------------------------")
	fmt.Printf("%v\n", decompressedContent)
	fmt.Println("---------------------------------------------------------")
	return FormatGitObjectContent(typ.ToGitType(), decompressedContent), nil
}

// Parses an OBJ_OFS_DELTA (offset delta object)
func parseOffsetDeltaObject(r io.Reader) ([]byte, error) {
	// Read variable-length offset (base object position)
	offset, err := readVariableLengthOffset(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read base offset: %w", err)
	}

	// Read and decompress delta instructions
	deltaData, err := ReadCompressed(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read delta data: %w", err)
	}

	fmt.Printf("[INFO] Read OFS_DELTA base offset: %d\n", offset)
	fmt.Printf("[INFO] Delta Data: %v\n", deltaData)

	// Delta application logic would go here (requires the base object)
	return deltaData, nil
}

// Parses an OBJ_REF_DELTA (reference delta object)
func parseRefDeltaObject(r io.Reader) ([]byte, error) {
	// Read 20-byte base object hash
	baseHash := [20]byte{}
	if _, err := io.ReadFull(r, baseHash[:]); err != nil {
		return nil, fmt.Errorf("failed to read base object hash: %w", err)
	}

	// Read and decompress delta instructions
	deltaData, err := ReadCompressed(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read delta data: %w", err)
	}

	fmt.Printf("[INFO] Read REF_DELTA base hash: %x\n", baseHash)
	fmt.Printf("[INFO] Delta Data: %v\n", deltaData)

	// Delta application logic would go here (requires the base object)
	return deltaData, nil
}

// Reads a variable-length offset (used in OBJ_OFS_DELTA)
func readVariableLengthOffset(r io.Reader) (int64, error) {
	var offset int64
	buf := [1]byte{}
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, fmt.Errorf("read offset byte: %w", err)
	}

	b := buf[0]
	offset = int64(b & 0x7F)

	for (b & 0x80) != 0 {
		offset += 1
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, fmt.Errorf("read offset byte: %w", err)
		}
		b = buf[0]
		offset = (offset << 7) | int64(b&0x7F)
	}
	return offset, nil
}
