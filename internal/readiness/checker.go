package readiness

import "context"

type Checker interface {
	Ping(context.Context) error
}

type Composite struct {
	checkers []Checker
}

func New(checkers ...Checker) Composite {
	return Composite{checkers: checkers}
}

func (c Composite) Ping(ctx context.Context) error {
	for _, checker := range c.checkers {
		if err := checker.Ping(ctx); err != nil {
			return err
		}
	}
	return nil
}
