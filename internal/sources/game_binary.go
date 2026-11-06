package sources

import (
	"encoding/binary"
	"errors"
	"unicode/utf8"
)

type gameBinary struct {
	data []byte
	pos  int
	err  error
}

func (r *gameBinary) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.data)-r.pos {
		r.err = errors.New("Truncated game file.")
		return nil
	}
	value := r.data[r.pos : r.pos+n]
	r.pos += n
	return value
}
func (r *gameBinary) u8() uint8 {
	v := r.take(1)
	if len(v) != 1 {
		return 0
	}
	return v[0]
}
func (r *gameBinary) u16() uint16 {
	v := r.take(2)
	if len(v) != 2 {
		return 0
	}
	return binary.LittleEndian.Uint16(v)
}
func (r *gameBinary) u32() uint32 {
	v := r.take(4)
	if len(v) != 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(v)
}
func (r *gameBinary) u64() uint64 {
	v := r.take(8)
	if len(v) != 8 {
		return 0
	}
	return binary.LittleEndian.Uint64(v)
}
func (r *gameBinary) count() int {
	v := r.u32()
	if v > 1000000 || uint64(v) > uint64(len(r.data)-r.pos) {
		r.err = errors.New("Invalid game record count.")
		return 0
	}
	return int(v)
}
func (r *gameBinary) text() string {
	var size uint32
	for shift := 0; shift <= 28; shift += 7 {
		b := r.u8()
		if shift == 28 && b > 7 {
			r.err = errors.New("Invalid game string length.")
			return ""
		}
		size |= uint32(b&127) << shift
		if b < 128 {
			if size > 1<<20 {
				r.err = errors.New("Game string is too long.")
				return ""
			}
			value := r.take(int(size))
			if !utf8.Valid(value) {
				r.err = errors.New("Invalid game text encoding.")
			}
			return string(value)
		}
	}
	r.err = errors.New("Invalid game string length.")
	return ""
}
func (r *gameBinary) finish() error {
	if r.err != nil {
		return r.err
	}
	if r.pos != len(r.data) {
		return errors.New("Unsupported trailing game data.")
	}
	return nil
}
