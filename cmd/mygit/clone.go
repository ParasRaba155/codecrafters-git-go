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

// errors
var (
	errInvalidPacketLineLength = errors.New("invalid git package length")
	packetLengthHeaderRegex    = regexp.MustCompile(`^[0-9a-f]{4}#`)
	objectIDRegex              = regexp.MustCompile(`[0-9a-f]{40}`)
	client                     = http.Client{
		Timeout: 5 * time.Second,
	}
)

const (
	service    = "service"
	uploadPack = "git-upload-pack"
	zeroID     = "0000000000000000000000000000000000000000"
	refName    = "HEAD"
)

// readPktLine returns the content after checking it's size, it return the size of the content
// It strips the first size 4 bytes of the result size, so the caller knows how much bytes it needs to read
// It checks for the special "0000" FLUSH-PACKET
func readPktLine(line []byte) (content []byte, size int, isFlushPkt bool, err error) {
	if len(line) <= 4 {
		return nil, len(line), false, fmt.Errorf("%w: length = %d", errInvalidPacketLineLength, len(line))
	}

	pktLength, err := strconv.ParseInt(string(line[:4]), 16, 64)
	if err != nil {
		return nil, int(pktLength), false, fmt.Errorf("read packet length: %w", err)
	}

	if pktLength == 0 {
		return nil, 0, true, nil
	}

	if pktLength == 4 {
		return nil, 0, false, fmt.Errorf("got packet size of 4")
	}

	if len(line) != int(pktLength) {
		return nil, int(pktLength), false, fmt.Errorf("mismatch in expected packet length, want %d got %d", pktLength, len(line))
	}
	// ignore the line feed char, if it exists
	if line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	return line[4:], int(pktLength) - 4, false, nil
}

// getPacketFile will
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

func validatePacketFile(body io.ReadCloser) error {
	defer func() {
		if err := body.Close(); err != nil {
			log.Printf("[ERROR] validatePacketFile close error: %v", err)
		}
	}()

	if err := validateHeader(body); err != nil {
		return err
	}

	return body.Close()
}

// validateHeader:
// the header is of the format: `4*(HEXDIGITS)# service=$servicename`
//
// the LF char at the end of line is optional. We can ignore it in our comparison.
// The 4 HEXDIGITS is the total length of the header itself including the optional LF char
// if it's present. The $servicename will be what we requested (e.g. in our case git-upload-pack)
func validateHeader(body io.Reader) error {
	buf := make([]byte, 5)
	_, err := io.ReadAtLeast(body, buf, len(buf))
	if err != nil {
		return fmt.Errorf("invalid packet header: %w", err)
	}
	if !packetLengthHeaderRegex.Match(buf) {
		return fmt.Errorf("invalid packet header: %s doesn't match the expected length header", buf)
	}
	headerLength, err := strconv.ParseInt(string(buf[:4]), 16, 64)
	if err != nil {
		return fmt.Errorf("parse the header length: %w", err)
	}
	// we have already read the first 5 bytes of the content, so we will only read the remaining in the line
	buf = make([]byte, headerLength-5)
	_, err = io.ReadAtLeast(body, buf, len(buf))
	if err != nil {
		return fmt.Errorf("read header: %w", err)
	}

	// ignore the trailing LF("\n") char
	if buf[len(buf)-1] == '\n' {
		buf = buf[:len(buf)-1]
	}
	if want := fmt.Sprintf(" %s=%s", service, uploadPack); !bytes.Equal(buf, []byte(want)) {
		return fmt.Errorf("packet header expectation failed, want: %q got: %q", want, buf)
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
	if !objectIDRegex.MatchString(want) {
		return nil, fmt.Errorf("invalid object id in the want: %q", want)
	}
	if !objectIDRegex.MatchString(have) {
		return nil, fmt.Errorf("invalid object id in the have: %q", have)
	}
	// the body is of the format
	// 0032want <obj-id>\n
	// 0032have <obj-id>\n
	// 0000
	// The '0000' is flush packet, and we should be fine with hard-coding the '0032'
	// since the length of given packet line for both the want and have will always be 50
	// and 50 in hex is 0032
	var b strings.Builder
	b.WriteString("0032 ")
	b.WriteString(want)
	b.WriteByte('\n')
	b.WriteString("0032 ")
	b.WriteString(have)
	b.WriteByte('\n')
	b.WriteString("0000")

	return strings.NewReader(b.String()), nil
}
