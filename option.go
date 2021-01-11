package appmain

import "context"

type Option interface {
	apply(c *config)
}

type optionFunc func(c *config)

func (f optionFunc) apply(c *config) {
	f(c)
}

type config struct {
	errorStrategy      ErrorStrategy
	defaultTaskOptions []TaskOption
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

func DefaultTaskOptions(opts ...TaskOption) Option {
	return optionFunc(func(c *config) {
		c.defaultTaskOptions = append(c.defaultTaskOptions, opts...)
	})
}
