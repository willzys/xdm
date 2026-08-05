package webauth

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type cdpClient struct {
	conn   net.Conn
	reader *bufio.Reader
	mu     sync.Mutex
	nextID int
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func dialCDP(ctx context.Context, value string) (*cdpClient, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "ws" || parsed.Hostname() == "" {
		return nil, errors.New("invalid browser debugging websocket URL")
	}
	if host := parsed.Hostname(); host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return nil, errors.New("refusing non-local browser debugging websocket")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", parsed.Host)
	if err != nil {
		return nil, err
	}
	keyBytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, keyBytes); err != nil {
		conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if parsed.RawQuery != "" {
		path += "?" + parsed.RawQuery
	}
	request := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + parsed.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if _, err := io.WriteString(conn, request); err != nil {
		conn.Close()
		return nil, err
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		conn.Close()
		return nil, err
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, fmt.Errorf("browser debugging websocket returned %s", response.Status)
	}
	if !strings.EqualFold(response.Header.Get("Upgrade"), "websocket") || !validWebsocketUpgrade(response.Header.Get("Connection")) {
		conn.Close()
		return nil, errors.New("browser debugging websocket returned invalid upgrade headers")
	}
	expected := sha1.Sum([]byte(key + websocketGUID))
	if response.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(expected[:]) {
		conn.Close()
		return nil, errors.New("browser debugging websocket handshake was rejected")
	}
	_ = conn.SetDeadline(time.Time{})
	return &cdpClient{conn: conn, reader: reader}, nil
}

func (c *cdpClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.writeFrame(0x8, nil)
	return c.conn.Close()
}

func (c *cdpClient) Call(ctx context.Context, method string, params any, target any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	id := c.nextID
	request := struct {
		ID     int    `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params,omitempty"`
	}{ID: id, Method: method, Params: params}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		return err
	}
	defer c.conn.SetDeadline(time.Time{})
	if err := c.writeFrame(0x1, payload); err != nil {
		return err
	}
	for {
		message, err := c.readMessage()
		if err != nil {
			return err
		}
		var response struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *cdpError       `json:"error"`
		}
		if err := json.Unmarshal(message, &response); err != nil || response.ID != id {
			continue
		}
		if response.Error != nil {
			return fmt.Errorf("browser debugging command %s failed: %s", method, response.Error.Message)
		}
		if target == nil || len(response.Result) == 0 {
			return nil
		}
		return json.Unmarshal(response.Result, target)
	}
}

func (c *cdpClient) readMessage() ([]byte, error) {
	var result []byte
	messageOpcode := byte(0)
	for {
		opcode, final, payload, err := c.readFrame()
		if err != nil {
			return nil, err
		}
		switch opcode {
		case 0x8:
			return nil, io.EOF
		case 0x9:
			if err := c.writeFrame(0xA, payload); err != nil {
				return nil, err
			}
			continue
		case 0xA:
			continue
		case 0x1, 0x2:
			if messageOpcode != 0 {
				return nil, errors.New("unexpected browser websocket data frame")
			}
			messageOpcode = opcode
			result = append(result, payload...)
		case 0x0:
			if messageOpcode == 0 {
				return nil, errors.New("unexpected browser websocket continuation")
			}
			result = append(result, payload...)
		default:
			continue
		}
		if len(result) > 16<<20 {
			return nil, errors.New("browser websocket response exceeds 16 MiB")
		}
		if final {
			if messageOpcode != 0x1 {
				return nil, errors.New("browser debugging returned a non-text message")
			}
			return result, nil
		}
	}
}

func (c *cdpClient) readFrame() (opcode byte, final bool, payload []byte, err error) {
	header := make([]byte, 2)
	if _, err = io.ReadFull(c.reader, header); err != nil {
		return 0, false, nil, err
	}
	final = header[0]&0x80 != 0
	opcode = header[0] & 0x0f
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		var extended [2]byte
		if _, err = io.ReadFull(c.reader, extended[:]); err != nil {
			return 0, false, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(extended[:]))
	case 127:
		var extended [8]byte
		if _, err = io.ReadFull(c.reader, extended[:]); err != nil {
			return 0, false, nil, err
		}
		length = binary.BigEndian.Uint64(extended[:])
	}
	if length > 16<<20 {
		return 0, false, nil, errors.New("browser websocket frame exceeds 16 MiB")
	}
	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(c.reader, mask[:]); err != nil {
			return 0, false, nil, err
		}
	}
	payload = make([]byte, int(length))
	if _, err = io.ReadFull(c.reader, payload); err != nil {
		return 0, false, nil, err
	}
	if masked {
		for index := range payload {
			payload[index] ^= mask[index%4]
		}
	}
	return opcode, final, payload, nil
}

func (c *cdpClient) writeFrame(opcode byte, payload []byte) error {
	header := []byte{0x80 | opcode}
	length := len(payload)
	switch {
	case length < 126:
		header = append(header, 0x80|byte(length))
	case length <= 65535:
		header = append(header, 0x80|126, byte(length>>8), byte(length))
	default:
		header = append(header, 0x80|127)
		var extended [8]byte
		binary.BigEndian.PutUint64(extended[:], uint64(length))
		header = append(header, extended[:]...)
	}
	var mask [4]byte
	if _, err := io.ReadFull(rand.Reader, mask[:]); err != nil {
		return err
	}
	header = append(header, mask[:]...)
	masked := make([]byte, len(payload))
	for index := range payload {
		masked[index] = payload[index] ^ mask[index%4]
	}
	if _, err := c.conn.Write(header); err != nil {
		return err
	}
	_, err := c.conn.Write(masked)
	return err
}

func validWebsocketUpgrade(value string) bool {
	for _, token := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
			return true
		}
	}
	return false
}
