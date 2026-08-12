package ngxstorage

import (
	"fmt"
	"strings"
)

// UnmarshalJSON decodes the NGX success acknowledgement. The backend has
// historically answered with a JSON boolean (true/false); newer releases
// encode the same flag as a string or numeric "1"/"0". All forms are accepted.
func (f *successFlag) UnmarshalJSON(data []byte) error {
	txt := strings.ToLower(strings.TrimSpace(string(data)))
	switch txt {
	case "true", `"true"`, "1", `"1"`:
		*f = true
	case "false", `"false"`, "0", `"0"`, `""`, "null":
		*f = false
	default:
		return fmt.Errorf("successFlag: unsupported value %s", data)
	}
	return nil
}
