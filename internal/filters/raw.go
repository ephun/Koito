package filters

import (
	"strconv"
	"strings"
)

func ValidEvidenceField(field string) bool {
	if !strings.HasPrefix(field, "evidence:") {
		return false
	}
	number := strings.TrimPrefix(field, "evidence:")
	id, err := strconv.ParseInt(number, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == number
}

func ValidItemField(field string) bool {
	return strings.HasPrefix(field, "item:") && ValidRawField("raw:"+strings.TrimPrefix(field, "item:"))
}

// Raw fields use JSON Pointer. Column fields address preserved CSV headers.
func ValidRawField(field string) bool {
	var path string
	if strings.HasPrefix(field, "raw:") {
		path = strings.TrimPrefix(field, "raw:")
	} else if strings.HasPrefix(field, "column:") {
		path = strings.TrimPrefix(field, "column:")
		if !strings.HasPrefix(path, "/") || strings.Count(path, "/") != 1 {
			return false
		}
	} else {
		return false
	}
	if len(path) > 2048 || path != "" && path[0] != '/' {
		return false
	}
	for i := 0; i < len(path); i++ {
		if path[i] == '~' {
			i++
			if i >= len(path) || path[i] != '0' && path[i] != '1' {
				return false
			}
		}
	}
	return true
}
