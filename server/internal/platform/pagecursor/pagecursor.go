// Package pagecursor encodes stable created-at/ID cursors for list endpoints.
package pagecursor

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrInvalid = errors.New("invalid pagination cursor")

type Value struct {
	CreatedAt time.Time
	ID        string
}

func Encode(createdAt time.Time, id string) string {
	value := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func Decode(encoded string) (Value, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return Value{}, ErrInvalid
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return Value{}, ErrInvalid
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return Value{}, ErrInvalid
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return Value{}, ErrInvalid
	}
	return Value{CreatedAt: createdAt.UTC(), ID: parts[1]}, nil
}
