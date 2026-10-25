package main

import (
	"encoding/binary"
	"errors"
	"io"
)

const maxFrame = 4 << 20

// Minimal bounded protobuf reader for the local Quasar stream. Only the
// wire types actually observed on this device are supported; anything else
// is rejected instead of guessed.
type pbValue struct {
	wire int
	num  uint64
	body []byte
}

type pbMessage map[int][]pbValue

func readFrame(r io.Reader) ([]byte, error) {
	var head [4]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(head[:])
	if size == 0 || size > maxFrame {
		return nil, errors.New("invalid frame size")
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func readVarint(b []byte, pos int) (uint64, int, error) {
	var v uint64
	for shift := uint(0); shift < 70; shift += 7 {
		if pos >= len(b) {
			return 0, pos, io.ErrUnexpectedEOF
		}
		c := b[pos]
		pos++
		if shift == 63 && c > 1 {
			return 0, pos, errors.New("varint overflow")
		}
		v |= uint64(c&0x7f) << shift
		if c < 0x80 {
			return v, pos, nil
		}
	}
	return 0, pos, errors.New("varint too long")
}

func parseMessage(b []byte) (pbMessage, error) {
	out := pbMessage{}
	for pos := 0; pos < len(b); {
		tag, next, err := readVarint(b, pos)
		if err != nil {
			return nil, err
		}
		pos = next
		num, wire := tag>>3, int(tag&7)
		if num == 0 {
			return nil, errors.New("zero field")
		}
		v := pbValue{wire: wire, num: num}
		switch wire {
		case 0:
			n, next, err := readVarint(b, pos)
			if err != nil {
				return nil, err
			}
			v.body, pos = []byte{byte(n)}, next
		case 1:
			if pos+8 > len(b) {
				return nil, io.ErrUnexpectedEOF
			}
			v.body, pos = b[pos:pos+8], pos+8
		case 2:
			size, next, err := readVarint(b, pos)
			if err != nil {
				return nil, err
			}
			if size > uint64(len(b)-next) {
				return nil, io.ErrUnexpectedEOF
			}
			v.body, pos = b[next:next+int(size)], next+int(size)
		case 5:
			if pos+4 > len(b) {
				return nil, io.ErrUnexpectedEOF
			}
			v.body, pos = b[pos:pos+4], pos+4
		default:
			return nil, errors.New("unsupported wire type")
		}
		out[int(num)] = append(out[int(num)], v)
	}
	return out, nil
}

func (m pbMessage) bytes(num int) []byte {
	vs := m[num]
	if len(vs) == 0 || vs[0].wire != 2 {
		return nil
	}
	return vs[0].body
}

func (m pbMessage) str(num int) string {
	return string(m.bytes(num))
}

func (m pbMessage) int(num int) (uint64, bool) {
	vs := m[num]
	if len(vs) == 0 || vs[0].wire != 0 || len(vs[0].body) != 1 {
		return 0, false
	}
	return uint64(vs[0].body[0]), true
}

func (m pbMessage) message(num int) pbMessage {
	b := m.bytes(num)
	if b == nil {
		return nil
	}
	sub, err := parseMessage(b)
	if err != nil {
		return nil
	}
	return sub
}

// aliceState describes one AliceState update observed on the local stream.
type aliceState struct {
	RequestID string
	Phrase    string
	Final     bool
	TTSText   string
	Response  []byte
}

func decodeAliceState(payload []byte) (*aliceState, bool) {
	root, err := parseMessage(payload)
	if err != nil {
		return nil, false
	}
	if kind, ok := root.int(253); !ok || kind != 3 {
		return nil, false
	}
	body := root.message(252)
	if body == nil {
		return nil, false
	}
	state := &aliceState{RequestID: body.str(7)}
	if asr := body.message(15); asr != nil {
		state.Phrase = asr.str(1)
		if final, ok := asr.int(2); ok {
			state.Final = final == 1
		}
	}
	if tts := body.message(14); tts != nil {
		state.TTSText = tts.str(1)
	}
	if resp := body.message(3); resp != nil {
		state.Response = resp.bytes(5)
	}
	return state, true
}
