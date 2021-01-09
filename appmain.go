package appmain

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

type App struct {
	sigChan <-chan os.Signal
	tasks   map[TaskType][]*task
	onError func(TaskContext) Decision
}

func New() *App {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	return &App{
		sigChan: sigChan,
		tasks:   make(map[TaskType][]*task),
		onError: func(tc TaskContext) Decision {
			err := tc.Err()
			if err == context.Canceled {
				return Continue
			}

			log.Printf("%s: %v", tc.Name(), err)
			switch tc.Type() {
			case TaskTypeInit, TaskTypeCleanup:
				return Continue
			case TaskTypeMain:
				return Exit
			default:
				panic("never happen")
			}
		},
	}
}

func (app *App) AddInitTask(name string, t Task) TaskContext {
	return app.addTask(name, TaskTypeInit, t)
}

func (app *App) AddMainTask(name string, t Task) TaskContext {
	return app.addTask(name, TaskTypeMain, t)
}

func (app *App) AddCleanupTask(name string, t Task) TaskContext {
	return app.addTask(name, TaskTypeCleanup, t)
}

func (app *App) addTask(name string, tt TaskType, t Task) TaskContext {
	r := newTask(name, tt, t)
	app.tasks[tt] = append(app.tasks[tt], r)
	return r
}

type Decision int

const (
	Continue Decision = iota + 1
	Exit
)

func (app *App) OnError(f func(tc TaskContext) Decision) {
	app.onError = f
}

func (app *App) Run() (code int) {
	var resultChan <-chan int
	var onceSignalReceived bool
	defer func() {
		ctx := context.Background()
		cleanupCtx, cancelCleanup := context.WithCancel(ctx)
		defer cancelCleanup()
		cleanupResult := app.cleanup(cleanupCtx)

		if resultChan != nil {
			select {
			case c := <-resultChan:
				if c != 0 && code != 0 {
					code = c
				}
			case <-app.sigChan:
				if code == 0 {
					code = 1
				}
				// exit immediately when received signal multiple times.
				if onceSignalReceived {
					return
				}
				onceSignalReceived = true
				return
			}
		}

		select {
		case c := <-cleanupResult:
			if c != 0 && code != 0 {
				code = c
			}
		case <-app.sigChan:
			if code == 0 {
				code = 1
			}
		}
	}()

	background := context.Background()
	initCtx, cancelInit := context.WithCancel(background)
	defer cancelInit()
	initResult := app.init(initCtx)
	resultChan = initResult

	select {
	case c := <-initResult:
		if c != 0 {
			return c
		}
	case <-app.sigChan:
		onceSignalReceived = true
		cancelInit()
		return 0
	}

	mainCtx, cancelMain := context.WithCancel(background)
	defer cancelMain()
	mainResult := app.main(mainCtx)
	resultChan = mainResult

	select {
	case c := <-mainResult:
		resultChan = nil
		return c
	case <-app.sigChan:
		cancelMain()
		onceSignalReceived = true
		return 0
	}
}

func (app *App) init(ctx context.Context) <-chan int {
	return app.runTask(ctx, TaskTypeInit)
}

func (app *App) main(ctx context.Context) <-chan int {
	return app.runTask(ctx, TaskTypeMain)
}

func (app *App) cleanup(ctx context.Context) <-chan int {
	return app.runTask(ctx, TaskTypeCleanup)
}

func (app *App) runTask(ctx context.Context, tt TaskType) <-chan int {
	tasks := app.tasks[tt]
	ctx, cancel := context.WithCancel(ctx)

	for _, t := range tasks {
		t := t
		go t.run(ctx)
	}

	result := make(chan int, 1)
	go func() {
		defer cancel()

		var code int
		for _, t := range tasks {
			<-t.Done()
			err := t.Err()
			if err != nil {
				decision := app.onError(t)
				if decision == Exit {
					code = 1
				}
			}
		}
		result <- code
	}()

	return result
}

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
}

func newTask(name string, tt TaskType, t Task) *task {
	return &task{
		name:  name,
		ttype: tt,
		task:  t,
		done:  make(chan struct{}),
		err:   nil,
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

func (t *task) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil && t.err != nil {
			t.err = fmt.Errorf("panic: %v", r)
		}
		close(t.done)
	}()

	t.err = t.task(ctx)
}
