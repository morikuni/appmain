package appmain

import (
	"os"
	"os/signal"
	"syscall"
)

// Option represents an interface of the option for the New function.
// Available options are below.
//   - ErrorStrategy
//   - DefaultTaskOptions
//   - NotifySignal
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
		errorStrategy: DefaultErrorStrategy,
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

// Decision is the result of ErrorStrategy used to decide if
// canceling current tasks.
type Decision int

const (
	// Continue indicates that keep running current tasks.
	Continue Decision = iota
	// Exit indicates that cancel current tasks.
	Exit
)

// ErrorStrategy is the option for the New function to decide how the App
// performs when any tasks return an error. It is called only if
// any tasks return an error. The error from the task is available
// from TaskContext.Err().
type ErrorStrategy func(TaskContext) Decision

func (s ErrorStrategy) apply(c *config) {
	c.errorStrategy = s
}

// DefaultErrorStrategy is the default strategy of the App.
func DefaultErrorStrategy(tc TaskContext) Decision {
	switch tc.Type() {
	case TaskTypeCleanup:
		return Continue
	case TaskTypeInit, TaskTypeMain:
		return Exit
	default:
		panic("never happen")
	}
}

// DefaultTaskOptions is the option for the New function to set the
// default option for every tasks in the App.
func DefaultTaskOptions(opts ...TaskOption) Option {
	return optionFunc(func(c *config) {
		c.defaultTaskOptions = append(c.defaultTaskOptions, opts...)
	})
}

// NotifySignal is the option for New function to overwrite the
// signals that the App handles to start cleanup tasks.
// By default, the App handles syscall.SIGINT and syscall.SIGTERM.
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
