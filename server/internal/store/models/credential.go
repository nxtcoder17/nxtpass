package models

import (
	"github.com/nxtcoder17/nxtpass/server/internal/ulid"
)

type Metadata struct {
	// CreatedBy denotes which peer has created this record
	CreatedBy string

	CreatedAt int64
	UpdatedAt int64
	DeletedAt int64
}

type Credential struct {
	ID ulid.ID

	Username string
	Password string

	Hosts []string

	Extra map[string]string

	Tags []string

	Namespace string

	Metadata `json:",inline"`
}
