package appmain

import "context"

type TaskOption func(c *taskConfig)

type taskConfig struct {
	after       []TaskContext
	interceptor Interceptor
}

func newTaskConfig(opts []TaskOption) *taskConfig {
	c := new(taskConfig)
	for _, o := range opts {
		o(c)
	}
	return c
}

func RunAfter(tc TaskContext) TaskOption {
	return func(c *taskConfig) {
		c.after = append(c.after, tc)
	}
}

type Interceptor func(context.Context, TaskContext, Task) error

func Intercept(is ...Interceptor) TaskOption {
	return func(c *taskConfig) {
		if c.interceptor == nil {
			is = append([]Interceptor{c.interceptor}, is...)
		}
		c.interceptor = joinInterceptors(is)
	}
}

func joinInterceptors(is []Interceptor) Interceptor {
	switch len(is) {
	case 0:
		panic("no interceptor")
	case 1:
		return is[0]
	default:
		head := is[0]
		tail := joinInterceptors(is[1:])
		return func(ctx1 context.Context, tc TaskContext, t Task) error {
			return head(ctx1, tc, func(ctx2 context.Context) error {
				return tail(ctx2, tc, t)
			})
		}
	}
}
