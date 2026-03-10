package store

import (
	"encoding/json"
	"strconv"

	storepb "github.com/usememos/memos/proto/gen/store"
	"google.golang.org/protobuf/encoding/protojson"
)

var (
	protojsonUnmarshaler = protojson.UnmarshalOptions{
		AllowPartial:   true,
		DiscardUnknown: true,
	}
)

// MarshalMemoPayload serializes a MemoPayload to JSON, ensuring that the
// `type` enum field is always written as its string name (e.g. "DAILY_LOG")
// rather than a numeric value. This is necessary because protojson relies on
// the compiled binary descriptor (rawDesc) which may not contain custom enum
// values added after the last `buf generate` run.
func MarshalMemoPayload(payload *storepb.MemoPayload) (string, error) {
	if payload == nil {
		return "{}", nil
	}
	bytes, err := protojson.Marshal(payload)
	if err != nil {
		return "", err
	}

	// If the type field was serialized as a number (unknown to rawDesc),
	// replace it with the correct string name from the Go-level enum map.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bytes, &raw); err != nil {
		return string(bytes), nil
	}
	if typeVal, ok := raw["type"]; ok {
		// Check if it was serialized as a number.
		s := string(typeVal)
		if num, err := strconv.Atoi(s); err == nil {
			if name, exists := storepb.MemoPayload_Type_name[int32(num)]; exists {
				raw["type"], _ = json.Marshal(name)
				fixed, err := json.Marshal(raw)
				if err == nil {
					return string(fixed), nil
				}
			}
		}
	}
	return string(bytes), nil
}

// RowStatus is the status for a row.
type RowStatus string

const (
	// Normal is the status for a normal row.
	Normal RowStatus = "NORMAL"
	// Archived is the status for an archived row.
	Archived RowStatus = "ARCHIVED"
)

func (r RowStatus) String() string {
	return string(r)
}
