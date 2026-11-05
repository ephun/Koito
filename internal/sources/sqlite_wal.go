package sources

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// SQLite's WAL checksums cover the header and each frame in order. Check the
// supplied sidecar before SQLite can silently fall back to an older checkpoint.
// Stale frames from an earlier reset are identified by different salts.
func validateWAL(mainPath, walPath string) error {
	file, err := os.Open(walPath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return nil
	}
	var header [32]byte
	if _, err = io.ReadFull(file, header[:]); err != nil {
		return fmt.Errorf("Incomplete SQLite WAL header.")
	}
	be := binary.BigEndian
	magic := be.Uint32(header[:4])
	if magic != 0x377f0682 && magic != 0x377f0683 || be.Uint32(header[4:8]) != 3007000 {
		return fmt.Errorf("Unsupported SQLite WAL version.")
	}
	pageSize := be.Uint32(header[8:12])
	if pageSize < 512 || pageSize > 65536 || pageSize&(pageSize-1) != 0 {
		return fmt.Errorf("Invalid SQLite WAL page size.")
	}
	db, err := os.Open(mainPath)
	if err != nil {
		return err
	}
	var dbHeader [18]byte
	_, err = io.ReadFull(db, dbHeader[:])
	db.Close()
	if err != nil {
		return err
	}
	dbPageSize := uint32(be.Uint16(dbHeader[16:]))
	if dbPageSize == 1 {
		dbPageSize = 65536
	}
	if dbPageSize != pageSize {
		return fmt.Errorf("SQLite database and WAL page sizes differ.")
	}
	var order binary.ByteOrder = binary.LittleEndian
	if magic == 0x377f0683 {
		order = binary.BigEndian
	}
	var a, b uint32
	checksum := func(data []byte) {
		for i := 0; i < len(data); i += 8 {
			a += order.Uint32(data[i:i+4]) + b
			b += order.Uint32(data[i+4:i+8]) + a
		}
	}
	checksum(header[:24])
	if a != be.Uint32(header[24:28]) || b != be.Uint32(header[28:32]) {
		return fmt.Errorf("SQLite WAL header checksum failed.")
	}
	frame := make([]byte, 24+pageSize)
	for {
		_, err = io.ReadFull(file, frame)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("Incomplete SQLite WAL frame.")
		}
		if !bytes.Equal(frame[8:16], header[16:24]) {
			return nil
		}
		checksum(frame[:8])
		checksum(frame[24:])
		if a != be.Uint32(frame[16:20]) || b != be.Uint32(frame[20:24]) {
			return fmt.Errorf("SQLite WAL frame checksum failed.")
		}
	}
}
