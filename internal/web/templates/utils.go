package templates

import (
	"encoding/json"
	"strings"

	"github.com/caedis/noreza/internal/mapping"
)

// jsonString marshals a value to JSON string, returning empty string on error
func jsonString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func concatKeys(keys []mapping.KeyMapping) string {
	var keyVals []string

	for _, v := range keys {
		if v.Mode == mapping.Mouse {
			keyVals = append(keyVals, mapping.CodeToMouse[v.Code])
		} else {
			keyVals = append(keyVals, mapping.CodeToKeyFriendly[v.Code])
		}
	}

	return strings.Join(keyVals, "\n")
}
