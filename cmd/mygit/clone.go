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

var errInvalidPacketLineLength = errors.New("invalid packet length")

var (
	objectIDRegex  = regexp.MustCompile(`[0-9a-f]{40}`)
	refRecordRegex = regexp.MustCompile(`([a-f0-9]{40})\srefs/(.*)`)
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

type RefRecord struct {
	ObjID string
	Name  string
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

// getPacketFile
func getPacketFile(repoURL string) (io.ReadCloser, error) {
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

// validatePacketFile validates the packet headers and body and returns the packet file format
func validatePacketFile(body io.ReadCloser) ([]PacketLine, error) {
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
	if err := validateHeader(result[0]); err != nil {
		return nil, fmt.Errorf("invalid packet header:%w", err)
	}

	return result, nil
}

// getAllRefs returns the RefRecords reading from the packet lines
// TODO: add error handling
func getAllRefs(pktLines []PacketLine) ([]RefRecord, error) {
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

// validateHeader:
// the header is of the format: `4*(HEXDIGITS)# service=$servicename`
func validateHeader(pktLine PacketLine) error {
	if want := fmt.Sprintf("# %s=%s", service, uploadPack); !bytes.Equal(pktLine.Content, []byte(want)) {
		return fmt.Errorf("packet header expectation failed, want: %q got: %q", want, pktLine.Content)
	}
	return nil
}

func discoverRef(repoURL string, want string, have string) (io.ReadCloser, error) {
	url, err := url.JoinPath(repoURL, uploadPack)
	if err != nil {
		return nil, fmt.Errorf("join path: %w", err)
	}

	contentType := fmt.Sprintf("application/x-%s-request", uploadPack)
	requestBody, err := createRefDiscovery(want, have)
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

func createRefDiscovery(want string, have string) (io.Reader, error) {
	// the body is of the format
	// 0032want <obj-id>\n
	// 0032have <obj-id>\n
	// 0000
	// The '0000' is flush packet, and we should be fine with hard-coding the '0032'
	// since the length of given packet line for both the want and have will always be 50
	// and 50 in hex is 0032
	var b strings.Builder
	b.WriteString("0032want ")
	b.WriteString(want)
	b.WriteByte('\n')
	if have != "" {
		b.WriteString("0032have ")
		b.WriteString(have)
		b.WriteByte('\n')
	}
	b.WriteString("0000")
	b.WriteString("0009done\n")

	return strings.NewReader(b.String()), nil
}
