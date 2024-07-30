package model

import "time"

type CFResourceState int

const (
	CFResourceStateUnknown CFResourceState = iota
	CFResourceStateReady
)

type CFResource struct {
	GUID      string     `json:"guid"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
	Metadata  Metadata   `json:"metadata"`
}
