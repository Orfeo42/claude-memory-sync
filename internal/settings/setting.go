package settings

import "time"

type Setting struct {
	Key       string    `json:"key"`
	Value     *string   `json:"value,omitempty"`
	Set       bool      `json:"set"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

type Version struct {
	Value     *string   `json:"value,omitempty"`
	Deleted   bool      `json:"deleted"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
}

type PutRequest struct {
	Key   Key
	Value string
	Actor string
}

type DeleteRequest struct {
	Key   Key
	Actor string
}
