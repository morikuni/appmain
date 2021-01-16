package appmain

import (
	"context"
)

// TaskOption represents an interface of the option for the Add***Task
// functions of App.
// Available TaskOption are below.
//   - RunAfter
//   - Interceptor
//   - ChainInterceptors
type TaskOption interface {
	applyTask(c *taskConfig)
}

type taskOptionFunc func(c *taskConfig)

func (f taskOptionFunc) applyTask(c *taskConfig) {
	f(c)
}

type taskConfig struct {
	after       []TaskContext
	interceptor Interceptor
}

func newTaskConfig(opts []TaskOption) *taskConfig {
	c := new(taskConfig)
	for _, o := range opts {
		o.applyTask(c)
	}
	return c
}

// RunAfter specifies dependent tasks that must be complete before executing the Task.
// The task will be executed even if dependent tasks exit with an error.
func RunAfter(tcs ...TaskContext) TaskOption {
	return taskOptionFunc(func(c *taskConfig) {
		c.after = append(c.after, tcs...)
	})
}

// Interceptor intercepts task to be executed.
type Interceptor func(context.Context, TaskContext, Task) error

func (i Interceptor) applyTask(c *taskConfig) {
	if c.interceptor != nil {
		c.interceptor = ChainInterceptors(append([]Interceptor{c.interceptor}, i)...)
	} else {
		c.interceptor = i
	}
}

// ChainInterceptors merges given slice of Interceptor into one Interceptor.
// The first Interceptor will be executed at the first and the last one is the last.
func ChainInterceptors(is ...Interceptor) Interceptor {
	switch len(is) {
	case 0:
		panic("no interceptor")
	case 1:
		return is[0]
	default:
		head := is[0]
		tail := ChainInterceptors(is[1:]...)
		return func(ctx1 context.Context, tc TaskContext, t Task) error {
			return head(ctx1, tc, func(ctx2 context.Context) error {
				return tail(ctx2, tc, t)
			})
		}
	}
}
