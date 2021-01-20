package appmain

import (
	"context"
	"errors"
	"fmt"
)

// Task represents a task in the App.
type Task func(ctx context.Context) error

// TaskType represents a type of the tasks.
type TaskType int

const (
	// TaskTypeInit indicates the task is init task.
	TaskTypeInit TaskType = iota + 1
	// TaskTypeMain indicates the task is main task.
	TaskTypeMain
	// TaskTypeCleanup indicates the task is cleanup task.
	TaskTypeCleanup
)

// TaskContext is the context of the Task.
type TaskContext interface {
	Name() string
	Type() TaskType
	Done() <-chan struct{}
	Err() error
}

type task struct {
	name   string
	ttype  TaskType
	task   Task
	done   chan struct{}
	err    error
	config *taskConfig
}

func newTask(name string, tt TaskType, t Task, opts []TaskOption) *task {
	config := newTaskConfig(opts)
	switch tt {
	case TaskTypeInit:
		for _, at := range config.after {
			if at.Type() != TaskTypeInit {
				panic(name + ": init task can run after only init task: " + at.Name())
			}
		}
	case TaskTypeMain:
		for _, at := range config.after {
			switch at.Type() {
			case TaskTypeInit:
				panic(name + ": main task always run after init task: " + at.Name())
			case TaskTypeCleanup:
				panic(name + ": main task should start before cleanup task: " + at.Name())
			}
		}
	}

	return &task{
		name:   name,
		ttype:  tt,
		task:   t,
		done:   make(chan struct{}),
		err:    nil,
		config: config,
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

func (t *task) skip() {
	t.err = ErrSkipped
	close(t.done)
}

func (t *task) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil && t.err == nil {
			t.err = fmt.Errorf("panic: %v", r)
		}
		close(t.done)
	}()

	for _, at := range t.config.after {
		<-at.Done()
	}

	if t.config.interceptor != nil {
		t.err = t.config.interceptor(ctx, t, t.task)
	} else {
		t.err = t.task(ctx)
	}
}

var ErrSkipped = errors.New("skipped")
