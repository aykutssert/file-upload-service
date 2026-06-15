package auth

import (
	"context"
	"errors"
)

var ErrInvalidAPIKey = errors.New("invalid API key")

type Resolver interface {
	Resolve(context.Context, string) (Principal, error)
}
