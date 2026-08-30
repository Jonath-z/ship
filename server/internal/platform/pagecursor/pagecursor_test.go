package pagecursor

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRoundTrip(t *testing.T) {
	createdAt := time.Date(2026, time.August, 30, 12, 30, 15, 123456000, time.UTC)
	id := uuid.NewString()

	decoded, err := Decode(Encode(createdAt, id))
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.CreatedAt.Equal(createdAt) || decoded.ID != id {
		t.Fatalf("decoded cursor = %#v", decoded)
	}
}

func TestDecodeRejectsInvalidCursor(t *testing.T) {
	for _, value := range []string{"", "not-base64", base64Value("bad|not-a-uuid")} {
		if _, err := Decode(value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Decode(%q) error = %v, want ErrInvalid", value, err)
		}
	}
}

func base64Value(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
