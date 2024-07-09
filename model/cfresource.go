package model

import "time"

type CFResourceStatus int

const (
	CFResourceStatusUnknown CFResourceStatus = iota
	CFResourceStatusReady
)

type CFResourceState struct {
	Status  CFResourceStatus
	Details string
}

type CFResource struct {
	GUID      string          `json:"guid"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt *time.Time      `json:"updated_at"`
	Metadata  Metadata        `json:"metadata"`
	State     CFResourceState `json:"state"`
}
