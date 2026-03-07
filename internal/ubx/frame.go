package ubx

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	SyncA = 0xB5
	SyncB = 0x62

	HeaderLen   = 6 // sync(2) + class(1) + id(1) + len(2)
	ChecksumLen = 2
)

var (
	ErrShortFrame   = errors.New("ubx: frame too short")
	ErrBadChecksum  = errors.New("ubx: checksum mismatch")
	ErrBadSync      = errors.New("ubx: bad sync bytes")
)

// Frame is a raw UBX frame with class, id, and payload.
type Frame struct {
	Class   uint8
	ID      uint8
	Payload []byte
}

// ClassID returns the combined class/id as a uint16 for switch dispatch.
func (f Frame) ClassID() uint16 {
	return uint16(f.Class)<<8 | uint16(f.ID)
}

// Checksum computes Fletcher-8 over class, id, length, and payload.
func Checksum(class, id uint8, payload []byte) (ckA, ckB uint8) {
	length := uint16(len(payload))
	var a, b uint8
	a += class; b += a
	a += id; b += a
	a += uint8(length); b += a
	a += uint8(length >> 8); b += a
	for _, v := range payload {
		a += v; b += a
	}
	return a, b
}

// Encode builds a complete UBX wire frame.
func Encode(class, id uint8, payload []byte) []byte {
	length := len(payload)
	buf := make([]byte, HeaderLen+length+ChecksumLen)
	buf[0] = SyncA
	buf[1] = SyncB
	buf[2] = class
	buf[3] = id
	binary.LittleEndian.PutUint16(buf[4:6], uint16(length))
	copy(buf[HeaderLen:], payload)
	ckA, ckB := Checksum(class, id, payload)
	buf[HeaderLen+length] = ckA
	buf[HeaderLen+length+1] = ckB
	return buf
}

// Decode parses a complete UBX wire frame (including sync bytes and checksum).
func Decode(raw []byte) (Frame, error) {
	if len(raw) < HeaderLen+ChecksumLen {
		return Frame{}, ErrShortFrame
	}
	if raw[0] != SyncA || raw[1] != SyncB {
		return Frame{}, ErrBadSync
	}
	class := raw[2]
	id := raw[3]
	length := binary.LittleEndian.Uint16(raw[4:6])
	total := HeaderLen + int(length) + ChecksumLen
	if len(raw) < total {
		return Frame{}, fmt.Errorf("%w: need %d, have %d", ErrShortFrame, total, len(raw))
	}
	payload := raw[HeaderLen : HeaderLen+int(length)]
	ckA, ckB := Checksum(class, id, payload)
	if raw[HeaderLen+int(length)] != ckA || raw[HeaderLen+int(length)+1] != ckB {
		return Frame{}, ErrBadChecksum
	}
	out := make([]byte, len(payload))
	copy(out, payload)
	return Frame{Class: class, ID: id, Payload: out}, nil
}

// EncodePoll builds a poll request (empty payload) for a given class/id.
func EncodePoll(class, id uint8) []byte {
	return Encode(class, id, nil)
}
