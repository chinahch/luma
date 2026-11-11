package httpapi

import (
	"encoding/json"
	"time"
)

// optionalTime distinguishes an omitted field (preserve) from null (clear).
type optionalTime struct {
	Set   bool
	Value *time.Time
}

func (o *optionalTime) UnmarshalJSON(data []byte) error {
	o.Set = true
	return json.Unmarshal(data, &o.Value)
}
