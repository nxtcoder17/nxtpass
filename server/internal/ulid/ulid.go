package ulid

import (
	"strings"

	"github.com/segmentio/ksuid"
)

type ID string

const (
	lenKSUIDBytes  = 20
	lenKSUIDString = 27
)

func ParseID(v string) (ID, error) {
	id := v[len(v)-lenKSUIDString:]
	k, err := ksuid.Parse(id)
	if err != nil {
		return "", err
	}

	return ID(v[0:len(v)-lenKSUIDString] + k.String()), nil
}

func (id ID) String() string {
	return string(id)
}

func New() ID {
	return ID(ksuid.New().String())
}

func NewWithPrefix(prefix string) ID {
	if !strings.HasPrefix(prefix, "-") {
		prefix = prefix + "-"
	}
	return ID(prefix + ksuid.New().String())
	// return append([]byte(prefix), ksuid.New().Bytes()...)
}
