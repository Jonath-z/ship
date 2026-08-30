package jsonfield

import (
	"encoding/json"
	"testing"
)

func TestNullableDistinguishesOmittedNullAndValue(t *testing.T) {
	type request struct {
		Port Nullable[int] `json:"port"`
	}
	var omitted request
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if omitted.Port.Set {
		t.Fatal("omitted field was marked as set")
	}
	var nullValue request
	if err := json.Unmarshal([]byte(`{"port":null}`), &nullValue); err != nil {
		t.Fatal(err)
	}
	if !nullValue.Port.Set || nullValue.Port.Value != nil {
		t.Fatalf("null field = %#v", nullValue.Port)
	}
	var value request
	if err := json.Unmarshal([]byte(`{"port":3000}`), &value); err != nil {
		t.Fatal(err)
	}
	if !value.Port.Set || value.Port.Value == nil || *value.Port.Value != 3000 {
		t.Fatalf("value field = %#v", value.Port)
	}
}
