// Package jsonfield distinguishes an omitted JSON field from an explicit null.
package jsonfield

import (
	"bytes"
	"encoding/json"
)

type Nullable[T any] struct {
	Set   bool
	Value *T
}

func (field *Nullable[T]) UnmarshalJSON(data []byte) error {
	field.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		field.Value = nil
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	field.Value = &value
	return nil
}
