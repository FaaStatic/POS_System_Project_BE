package paginated

import (
	"time"

	"github.com/google/uuid"
)

type CursorMeta struct {
	AfterTime time.Time `json:"after_time"`
	AfterID   uuid.UUID `json:"after_id"`
}

const TimeLayout = time.RFC3339Nano

func (c *CursorMeta) Encode() (afterID string, afterTime string) {
	if c == nil {
		return "", ""
	}
	return c.AfterID.String(), c.AfterTime.Format(TimeLayout)
}
