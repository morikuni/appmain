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
	sigChan chan os.Signal
	tasks   map[TaskType][]*task
	onError func(TaskContext) Decision
}

func New() *App {
	return &App{
		sigChan: nil,
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

func (app *App) AddInitTask(name string, t Task, opts ...TaskOption) TaskContext {
	return app.addTask(name, TaskTypeInit, t, opts)
}

func (app *App) AddMainTask(name string, t Task, opts ...TaskOption) TaskContext {
	return app.addTask(name, TaskTypeMain, t, opts)
}

func (app *App) AddCleanupTask(name string, t Task, opts ...TaskOption) TaskContext {
	return app.addTask(name, TaskTypeCleanup, t, opts)
}

func (app *App) addTask(name string, tt TaskType, t Task, opts []TaskOption) TaskContext {
	r := newTask(name, tt, t, opts)
	app.tasks[tt] = append(app.tasks[tt], r)
	return r
}

type Decision int

const (
	Continue Decision = iota
	Exit
)

func (app *App) OnError(f func(tc TaskContext) Decision) {
	app.onError = f
}

func (app *App) ShutdownOnSignal(sigs ...os.Signal) {
	if app.sigChan != nil {
		signal.Stop(app.sigChan)
	}
	app.sigChan = make(chan os.Signal, 1)
	signal.Notify(app.sigChan, sigs...)
}

func (app *App) SendSignal(sig os.Signal) {
	if app.sigChan != nil {
		app.sigChan <- sig
	}
}

func (app *App) Run() (code int) {
	if app.sigChan == nil {
		app.ShutdownOnSignal(os.Interrupt, syscall.SIGTERM)
	}

	var (
		resultChan  <-chan int
		signalCount int
	)
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
			case sig := <-app.sigChan:
				if code == 0 {
					code = signalCode(sig)
				}

				// When this if block is executing, since resultChan it not nil,
				// there was a cancel of either of main or init execution by signal.
				// Therefore signalCount must be 1 and this is 2nd time of signal,
				// so exit immediately without waiting cleanup result.
				return
			}
		}

		for {
			select {
			case c := <-cleanupResult:
				if c != 0 && code == 0 {
					code = c
				}
				return
			case sig := <-app.sigChan:
				signalCount++
				if signalCount >= 2 {
					if code == 0 {
						code = signalCode(sig)
					}
					return
				}
				cancelCleanup()
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
		signalCount++
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
		signalCount++
		cancelMain()
		return 0
	}
}
func signalCode(sig os.Signal) int {
	s, ok := sig.(syscall.Signal)
	if ok {
		// It should exit with 128 + <signal code>.
		// https://tldp.org/LDP/abs/html/exitcodes.html
		return int(s) + 128
	}
	return 1
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
		config: newTaskConfig(opts),
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

	for _, at := range t.config.after {
		<-at.Done()
	}

	t.err = t.task(ctx)
}

type taskConfig struct {
	after []TaskContext
}

func newTaskConfig(opts []TaskOption) *taskConfig {
	c := new(taskConfig)
	for _, o := range opts {
		o(c)
	}
	return c
}

type TaskOption func(c *taskConfig)

func RunAfter(tc TaskContext) TaskOption {
	return func(c *taskConfig) {
		c.after = append(c.after, tc)
	}
}
