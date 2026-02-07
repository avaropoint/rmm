package protocol

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
)

// WebSocket GUID per RFC 6455 section 4.2.2.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// AcceptKey computes the Sec-WebSocket-Accept value for a given key.
func AcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// ReadFrame reads a single WebSocket frame from r.
// It handles extended payload lengths and optional masking.
func ReadFrame(r *bufio.Reader) (opcode byte, payload []byte, err error) {
	header := make([]byte, 2)
	if _, err = io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}

	opcode = header[0] & 0x0F
	masked := (header[1] & 0x80) != 0
	length := uint64(header[1] & 0x7F)

	switch length {
	case 126:
		ext := make([]byte, 2)
		if _, err = io.ReadFull(r, ext); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		if _, err = io.ReadFull(r, ext); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(ext)
	}

	var maskKey []byte
	if masked {
		maskKey = make([]byte, 4)
		if _, err = io.ReadFull(r, maskKey); err != nil {
			return 0, nil, err
		}
	}

	payload = make([]byte, length)
	if _, err = io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}

	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return opcode, payload, nil
}

// WriteServerFrame writes an unmasked WebSocket frame (server → client).
func WriteServerFrame(conn net.Conn, opcode byte, payload []byte) error {
	length := len(payload)

	// Pre-allocate: 2-byte header + up to 8 extended length bytes + payload
	frame := make([]byte, 0, 2+8+length)
	frame = append(frame, 0x80|opcode)

	switch {
	case length < 126:
		frame = append(frame, byte(length))
	case length < 65536:
		frame = append(frame, 126, byte(length>>8), byte(length))
	default:
		frame = append(frame, 127)
		for i := 7; i >= 0; i-- {
			frame = append(frame, byte(length>>(i*8)))
		}
	}

	frame = append(frame, payload...)
	_, err := conn.Write(frame)
	return err
}

// WriteClientFrame writes a masked WebSocket frame (client → server).
func WriteClientFrame(conn net.Conn, opcode byte, payload []byte) error {
	length := len(payload)

	// Pre-allocate: 2-byte header + up to 8 extended + 4 mask + payload
	frame := make([]byte, 0, 2+8+4+length)
	frame = append(frame, 0x80|opcode)

	switch {
	case length < 126:
		frame = append(frame, byte(length)|0x80)
	case length < 65536:
		frame = append(frame, 126|0x80, byte(length>>8), byte(length))
	default:
		frame = append(frame, 127|0x80)
		for i := 7; i >= 0; i-- {
			frame = append(frame, byte(length>>(i*8)))
		}
	}

	maskKey := [4]byte{}
	rand.Read(maskKey[:]) //nolint:errcheck
	frame = append(frame, maskKey[:]...)

	// Mask inline into the same allocation
	off := len(frame)
	frame = frame[:off+length]
	for i, b := range payload {
		frame[off+i] = b ^ maskKey[i&3]
	}

	_, err := conn.Write(frame)
	return err
}
