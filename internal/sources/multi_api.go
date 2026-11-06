package sources

import (
	"encoding/json"
)

const MultiAPI = "multi-scrobbler-api-v1"

func parseMultiAPI(file File, data []byte) (parsedFile, error) {
	part := parsedFile{file: file, service: "multi-scrobbler", parser: MultiAPI}
	var page struct {
		Data []json.RawMessage `json:"data"`
		Meta *struct {
			Total  int `json:"total"`
			Limit  int `json:"limit"`
			Offset int `json:"offset"`
		} `json:"meta"`
	}
	if json.Unmarshal(data, &page) != nil || page.Meta == nil || page.Data == nil {
		return part, ErrUnsupportedFile
	}
	if len(page.Data) == 0 {
		return part, ErrUnsupportedFile
	}
	for _, raw := range page.Data {
		var row map[string]json.RawMessage
		if json.Unmarshal(raw, &row) != nil || row["play"] == nil || row["componentId"] == nil || row["uid"] == nil {
			return part, ErrUnsupportedFile
		}
	}
	part.records = page.Data
	return part, nil
}

func normalizeMultiAPI(raw json.RawMessage) recordValues {
	var row map[string]any
	_ = json.Unmarshal(raw, &row)
	v := normalizeMultiPlay(row["play"])
	v.Forwarder = "multi-scrobbler"
	if uid := valueText(row["uid"]); uid != "" {
		v.EventID = "multi:" + uid
	}
	parent := object(row["parent"])
	if row["parentId"] != nil || parent != nil {
		v.Reason = "forwarding evidence"
		if uid := valueText(parent["uid"]); uid != "" {
			v.SupportID = "multi:" + uid
		}
	} else if valueText(row["state"]) == "duped" {
		v.Reason = "source duplicate"
	}
	return v
}
