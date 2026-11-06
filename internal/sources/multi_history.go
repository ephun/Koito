package sources

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const MultiCache = "multi-scrobbler-cache-v1"
const MultiLog = "multi-scrobbler-log-v1"

// Flatted references apply to strings inside containers, not to the contents of
// a referenced string. Refuse cycles here rather than expand them indefinitely.
func unflatten(pool []any, index int, active map[int]bool) (any, error) {
	if index < 0 || index >= len(pool) || active[index] {
		return nil, fmt.Errorf("Unsupported Flatted reference.")
	}
	active[index] = true
	defer delete(active, index)
	var expand func(any) (any, error)
	expand = func(value any) (any, error) {
		switch item := value.(type) {
		case string:
			ref, err := strconv.Atoi(item)
			if err != nil {
				return nil, fmt.Errorf("Invalid Flatted reference.")
			}
			return unflatten(pool, ref, active)
		case []any:
			out := make([]any, len(item))
			for i, v := range item {
				var err error
				out[i], err = expand(v)
				if err != nil {
					return nil, err
				}
			}
			return out, nil
		case map[string]any:
			out := map[string]any{}
			for key, v := range item {
				var err error
				out[key], err = expand(v)
				if err != nil {
					return nil, err
				}
			}
			return out, nil
		default:
			return value, nil
		}
	}
	if _, primitive := pool[index].(string); primitive {
		return pool[index], nil
	}
	return expand(pool[index])
}

func typesonPath(root any, path string) any {
	value := root
	for _, key := range strings.Split(strings.TrimPrefix(path, "#"), ".") {
		switch current := value.(type) {
		case map[string]any:
			value = current[key]
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(current) {
				return nil
			}
			value = current[index]
		default:
			return nil
		}
	}
	return value
}

func decodeTypeson(root map[string]any) (any, bool) {
	types := object(root["$types"])
	for _, value := range types {
		switch valueText(value) {
		case "arrayNonindexKeys", "undef", "Dayjs", "ListenProgressPositional", "#":
		default:
			return nil, false
		}
	}
	active := map[string]bool{}
	var read func(any, string, int) any
	read = func(value any, path string, depth int) any {
		if depth > 100 || active[path] {
			return map[string]any{"$reference": path}
		}
		if valueText(types[path]) == "undef" {
			return nil
		}
		if valueText(types[path]) == "#" {
			active[path] = true
			defer delete(active, path)
			target := strings.TrimPrefix(valueText(value), "#")
			return read(typesonPath(root, target), target, depth+1)
		}
		switch current := value.(type) {
		case []any:
			out := make([]any, len(current))
			for i, item := range current {
				out[i] = read(item, path+"."+strconv.Itoa(i), depth+1)
			}
			return out
		case map[string]any:
			out := map[string]any{}
			for key, item := range current {
				out[key] = read(item, path+"."+key, depth+1)
			}
			return out
		default:
			return value
		}
	}
	return read(root["value"], "value", 0), true
}

func parseMultiHistory(file File, data []byte, filename string) (parsedFile, error) {
	part := parsedFile{file: file, service: "multi-scrobbler"}
	if strings.HasSuffix(strings.ToLower(filename), ".cache") {
		var pool []any
		if json.Unmarshal(data, &pool) != nil || len(pool) == 0 {
			return part, ErrUnsupportedFile
		}
		decoded, err := unflatten(pool, 0, map[int]bool{})
		if err != nil {
			return part, err
		}
		entries, ok := decoded.([]any)
		if !ok {
			return part, ErrUnsupportedFile
		}
		part.parser = MultiCache
		for _, entry := range entries {
			item := object(entry)
			key := valueText(item["key"])
			if !strings.HasPrefix(key, "keyv:") {
				return part, ErrUnsupportedFile
			}
			encoded := jsonObject(item["value"])
			values, supported := decodeTypeson(encoded)
			manifest, _ := json.Marshal(map[string]any{"kind": "cache", "key": key, "encoded": item["value"], "supported": supported})
			part.records = append(part.records, manifest)
			if !supported {
				continue
			}
			rows, ok := values.([]any)
			if !ok {
				continue
			}
			for _, value := range rows {
				row := object(value)
				play := object(row["play"])
				original := object(object(object(play["meta"])["lifecycle"])["original"])
				if original == nil {
					original = play
				}
				raw, _ := json.Marshal(map[string]any{"kind": "cache_play", "key": key, "entry": row, "play": play, "source_input": original})
				part.records = append(part.records, raw)
			}
		}
		return part, nil
	}
	name := strings.ToLower(baseFilename(filename))
	if !strings.HasSuffix(name, ".log") || !(strings.HasPrefix(name, "scrobble-") || strings.HasPrefix(name, "multi-scrobbler")) {
		return part, ErrUnsupportedFile
	}
	part.parser = MultiLog
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 32<<20)
	for scanner.Scan() {
		line := scanner.Text()
		row := map[string]any{"kind": "log", "line": line}
		if match := multiLogPlay.FindStringSubmatch(line); len(match) > 1 {
			row["play_uid"] = match[1]
		}
		var fields map[string]any
		if json.Unmarshal([]byte(line), &fields) == nil && fields != nil {
			row["fields"] = fields
		}
		raw, _ := json.Marshal(row)
		part.records = append(part.records, raw)
	}
	if scanner.Err() != nil {
		return part, scanner.Err()
	}
	if len(part.records) == 0 {
		part.records = []json.RawMessage{json.RawMessage(`{"kind":"log","empty":true}`)}
	}
	return part, nil
}

var multiLogPlay = regexp.MustCompile(`\[Play ([A-Za-z0-9_-]+)\]`)

func normalizeMultiHistory(raw json.RawMessage, parser string) recordValues {
	var row map[string]any
	_ = json.Unmarshal(raw, &row)
	v := recordValues{Reason: "forwarding evidence", Origin: "forwarded", Forwarder: "multi-scrobbler"}
	if parser == MultiLog {
		if uid := valueText(row["play_uid"]); uid != "" {
			v.SupportID = "multi:" + uid
		}
		return v
	}
	if valueText(row["kind"]) != "cache_play" {
		return v
	}
	v = normalizeMultiPlay(row["play"])
	v.Forwarder = "multi-scrobbler"
	origin := normalizeMultiPlay(row["source_input"])
	if origin.Time.IsZero() || origin.Title == "" || origin.Artist == "" {
		origin = v
	}
	identity, _ := json.Marshal([]any{origin.OriginalService, origin.Device, origin.TimeText, origin.Title, origin.Artist, origin.Album, origin.URI})
	v.EventID = "multi-cache:" + digest(identity)
	return v
}
