package ngxstorage

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FlexBool decodes the canonical NGX boolean encodings: JSON boolean,
// "true"/"false" strings, numeric "1"/"0", and the live NFS export
// representation "1" enabled / "" disabled. All forms are accepted.
type FlexBool bool

// UnmarshalJSON accepts every documented NGX boolean encoding.
func (f *FlexBool) UnmarshalJSON(data []byte) error {
	parsed, err := flexBoolFromValue(decodeJSONValue(data))
	if err != nil {
		return err
	}
	*f = parsed
	return nil
}

// successFlag is the canonical mutation acknowledgement. It reuses FlexBool
// so every documented backend encoding (boolean, "1"/"0", "true"/"false")
// is accepted by mutation responses.
type successFlag = FlexBool

// FlexBytes decodes a canonical NGX byte value that the backend serializes
// either as a JSON number or as a decimal string. Null and empty values are
// treated as zero.
type FlexBytes int64

// UnmarshalJSON accepts numeric and string byte encodings.
func (f *FlexBytes) UnmarshalJSON(data []byte) error {
	parsed, err := flexBytesFromValue(decodeJSONValue(data))
	if err != nil {
		return err
	}
	*f = parsed
	return nil
}

// flexBoolFromValue decodes one already-parsed JSON value into a FlexBool.
func flexBoolFromValue(v interface{}) (FlexBool, error) {
	txt := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v)))
	switch txt {
	case "true", "1":
		return true, nil
	case "false", "0", "", "<nil>", "null":
		return false, nil
	default:
		return false, fmt.Errorf("ngxstorage: FlexBool unsupported value %q", v)
	}
}

// flexBytesFromValue decodes one already-parsed JSON value into a FlexBytes.
func flexBytesFromValue(v interface{}) (FlexBytes, error) {
	switch raw := v.(type) {
	case nil:
		return 0, nil
	case float64:
		return FlexBytes(int64(raw)), nil
	case json.Number:
		n, err := raw.Int64()
		return FlexBytes(n), err
	case string:
		text := strings.TrimSpace(raw)
		if text == "" {
			return 0, nil
		}
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("ngxstorage: FlexBytes unsupported value %q", raw)
		}
		return FlexBytes(n), nil
	default:
		return 0, fmt.Errorf("ngxstorage: FlexBytes unsupported value %q", v)
	}
}

// decodeJSONValue parses a raw JSON token into a value suitable for the
// flexible decoders. json.Unmarshal already produced interface{} values in
// struct contexts; this helper is used by the direct UnmarshalJSON methods.
func decodeJSONValue(data []byte) interface{} {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return string(data)
	}
	return v
}
