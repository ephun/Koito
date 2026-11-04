package filters

import "strings"

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
