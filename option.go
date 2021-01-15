package appmain

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

type Option interface {
	apply(c *config)
}

type optionFunc func(c *config)

func (f optionFunc) apply(c *config) {
	f(c)
}

type config struct {
	sigChan            chan os.Signal
	sigSet             map[os.Signal]struct{}
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

	if c.sigChan == nil {
		c.sigChan = make(chan os.Signal, 1)
		signal.Notify(c.sigChan, os.Interrupt, syscall.SIGTERM)
		c.sigSet = map[os.Signal]struct{}{
			os.Interrupt:    {},
			syscall.SIGTERM: {},
		}
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
	case TaskTypeCleanup:
		return Continue
	case TaskTypeInit, TaskTypeMain:
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

func NotifySignal(sigs ...os.Signal) Option {
	return optionFunc(func(c *config) {
		c.sigChan = make(chan os.Signal, 1)
		signal.Notify(c.sigChan, sigs...)
		set := make(map[os.Signal]struct{}, len(sigs))
		for _, s := range sigs {
			set[s] = struct{}{}
		}
		c.sigSet = set
	})
}
