package sources

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

func (p parsedFile) countRecords() int {
	if p.recordFile != "" {
		return p.recordCount
	}
	return len(p.records)
}

func (p parsedFile) eachRecord(fn func(int, json.RawMessage) error) error {
	if p.recordFile == "" {
		for i, raw := range p.records {
			if err := fn(i, raw); err != nil {
				return err
			}
		}
		return nil
	}
	f, err := os.Open(p.recordFile)
	if err != nil {
		return err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	for i := 0; ; i++ {
		var raw json.RawMessage
		err = d.Decode(&raw)
		if err == io.EOF {
			if i != p.recordCount {
				return fmt.Errorf("Incomplete record extraction.")
			}
			return nil
		}
		if err != nil {
			return err
		}
		if i >= p.recordCount {
			return fmt.Errorf("Record extraction contains unexpected data.")
		}
		if err = fn(i, raw); err != nil {
			return err
		}
	}
}

func cleanupParsedFiles(parts []parsedFile) {
	for _, part := range parts {
		if part.recordFile != "" {
			_ = os.Remove(part.recordFile)
		}
	}
}
