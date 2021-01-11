package appmain

import (
	"context"
	"fmt"
)

type Task func(ctx context.Context) error

type TaskType int

const (
	TaskTypeInit TaskType = iota + 1
	TaskTypeMain
	TaskTypeCleanup
)

type TaskContext interface {
	Name() string
	Type() TaskType
	Done() <-chan struct{}
	Err() error
}

type task struct {
	name  string
	ttype TaskType
	task  Task
	done  chan struct{}
	err   error
	opts  []TaskOption
}

func newTask(name string, tt TaskType, t Task, opts []TaskOption) *task {
	return &task{
		name:  name,
		ttype: tt,
		task:  t,
		done:  make(chan struct{}),
		err:   nil,
		opts:  opts,
	}
}

func (t *task) Name() string {
	return t.name
}

func (t *task) Type() TaskType {
	return t.ttype
}

func (t *task) Done() <-chan struct{} {
	return t.done
}

func (t *task) Err() error {
	return t.err
}

func (t *task) run(ctx context.Context, defaults []TaskOption) {
	config := newTaskConfig(append(defaults, t.opts...))
	switch t.ttype {
	case TaskTypeInit:
		for _, at := range config.after {
			if at.Type() != TaskTypeInit {
				panic(t.name + ": init task can run after only init task: " + at.Name())
			}
		}
	case TaskTypeMain:
		for _, at := range config.after {
			switch at.Type() {
			case TaskTypeInit:
				panic(t.name + ": main task always run after init task: " + at.Name())
			case TaskTypeCleanup:
				panic(t.name + ": main task should start before cleanup task: " + at.Name())
			}
		}
	}

	defer func() {
		if r := recover(); r != nil && t.err != nil {
			t.err = fmt.Errorf("panic: %v", r)
		}
		close(t.done)
	}()

	for _, at := range config.after {
		<-at.Done()
	}

	if config.interceptor != nil {
		t.err = config.interceptor(ctx, t, t.task)
	} else {
		t.err = t.task(ctx)
	}
}
