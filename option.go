package appmain

import "context"

type Option interface {
	apply(c *config)
}

type config struct {
	errorStrategy ErrorStrategy
}

func newConfig(opts []Option) *config {
	c := &config{
		errorStrategy: defaultErrorStrategy,
	}

	for _, o := range opts {
		o.apply(c)
	}
	return c
}

type Decision int

const (
	Continue Decision = iota
	Exit
)

type ErrorStrategy func(TaskContext) Decision

func (s ErrorStrategy) apply(c *config) {
	c.errorStrategy = s
}

func defaultErrorStrategy(tc TaskContext) Decision {
	err := tc.Err()
	if err == context.Canceled {
		return Continue
	}

	switch tc.Type() {
	case TaskTypeInit, TaskTypeCleanup:
		return Continue
	case TaskTypeMain:
		return Exit
	default:
		panic("never happen")
	}
}
