package id

import "github.com/google/uuid"

type Generator interface {
	New() (string, error)
}

type UUIDv7 struct{}

func (UUIDv7) New() (string, error) {
	value, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return value.String(), nil
}
