package comments

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/akhmed9505/comment-tree/internal/domain"
)

// decodeObject rejects duplicate keys, unknown fields and trailing JSON values.
func decodeObject(data []byte, value any) error {
	if !utf8.Valid(data) {
		return domain.ErrInvalidInput
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return domain.ErrInvalidInput
	}
	allowed := map[string]bool{}
	typ := reflect.TypeOf(value).Elem()
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			allowed[name] = true
		}
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return domain.ErrInvalidInput
		}
		key, ok := token.(string)
		if !ok || !allowed[key] || seen[key] {
			return domain.ErrInvalidInput
		}
		seen[key] = true
		var raw json.RawMessage
		if d.Decode(&raw) != nil {
			return domain.ErrInvalidInput
		}
	}
	if _, err := d.Token(); err != nil {
		return domain.ErrInvalidInput
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return domain.ErrInvalidInput
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return domain.ErrInvalidInput
	}
	return nil
}
