package readiness

import (
	"context"
	"errors"
	"testing"
)

type stubChecker struct {
	err error
}

func (s stubChecker) Ping(context.Context) error {
	return s.err
}

func TestComposite(t *testing.T) {
	tests := []struct {
		name     string
		checkers []Checker
		wantErr  bool
	}{
		{
			name:     "all available",
			checkers: []Checker{stubChecker{}, stubChecker{}},
		},
		{
			name: "dependency unavailable",
			checkers: []Checker{
				stubChecker{},
				stubChecker{err: errors.New("unavailable")},
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := New(test.checkers...).Ping(context.Background())
			if (err != nil) != test.wantErr {
				t.Fatalf("Ping() error = %v", err)
			}
		})
	}
}
