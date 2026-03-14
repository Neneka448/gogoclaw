package memory

import "time"

type MemoryEdge struct {
	SourceID  string    `json:"source_id"`
	TargetID  string    `json:"target_id"`
	Weight    float64   `json:"weight"`
	CreatedAt time.Time `json:"created_at"`
}
