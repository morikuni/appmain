package appmain

import (
	"context"
	"os"
	"syscall"
)

// App represents an application.
type App struct {
	tasks  map[TaskType][]*task
	config *config
}

// New creates a new App instance from options.
// Available options are below.
//   - ErrorStrategy
//   - DefaultTaskOptions
//   - NotifySignal
func New(opts ...Option) *App {
	return &App{
		tasks:  make(map[TaskType][]*task),
		config: newConfig(opts),
	}
}

// AddInitTask add a task with name as initialization task of the App.
//
// The init tasks are executed before starting main tasks. Therefore,
// the main tasks won't start until all init tasks complete.
//
// By default, if any init task return an error, the App will cancels all
// context.Context in all init tasks and start cleanup tasks before executing
// main tasks. To overwrite default strategy, use ErrorStrategy option to the New function.
func (app *App) AddInitTask(name string, t Task, opts ...TaskOption) TaskContext {
	return app.addTask(name, TaskTypeInit, t, opts)
}

// AddMainTask add a task with name as main task of the App.
//
// The main tasks will start after all init tasks completed.
//
// By default, if any main task return an error, the App will cancels all
// context.Context in all  main tasks and start cleanup tasks.
// To overwrite default strategy, use ErrorStrategy option to the New function.
func (app *App) AddMainTask(name string, t Task, opts ...TaskOption) TaskContext {
	return app.addTask(name, TaskTypeMain, t, opts)
}

// AddCleanupTask add a task with name as cleanup task of the App.
//
// The cleanup tasks are executed when all main tasks completed or the first signal
// is received. If signal is received twice, the context.Context in cleanup tasks is
// canceled and the App exit without waiting completion of cleanup tasks.
//
// Unlike init and main tasks, context.Context in the cleanup tasks is not canceled
// even if some tasks return error.
func (app *App) AddCleanupTask(name string, t Task, opts ...TaskOption) TaskContext {
	return app.addTask(name, TaskTypeCleanup, t, opts)
}

func (app *App) addTask(name string, tt TaskType, t Task, opts []TaskOption) TaskContext {
	r := newTask(name, tt, t, append(app.config.defaultTaskOptions, opts...))
	app.tasks[tt] = append(app.tasks[tt], r)
	return r
}

// SendSignal performs like sending a signal to the App.
// Since the App handles signal by default, this method is not necessary for usual apps.
// It is useful for testing the App.
func (app *App) SendSignal(sig os.Signal) {
	if _, ok := app.config.sigSet[sig]; ok {
		app.config.sigChan <- sig
	}
}

// Run runs the App and returns status code for the main function.
// It can be used like following.
//
//   os.Exit(app.Run())
func (app *App) Run() (code int) {
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
			case sig := <-app.config.sigChan:
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
			case sig := <-app.config.sigChan:
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
			resultChan = nil
			app.skipMain()
			return c
		}
	case <-app.config.sigChan:
		signalCount++
		cancelInit()
		app.skipMain()
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
	case <-app.config.sigChan:
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

func (app *App) skipMain() {
	for _, t := range app.tasks[TaskTypeMain] {
		t.skip()
	}
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

	doneTCs := make(chan TaskContext, len(tasks))
	for _, t := range tasks {
		t := t
		go func() {
			t.run(ctx)
			doneTCs <- t
		}()
	}

	result := make(chan int, 1)
	go func() {
		defer cancel()

		var code int
		for i := 0; i < len(tasks); i++ {
			tc := <-doneTCs
			err := tc.Err()
			if err != nil {
				decision := app.config.errorStrategy(tc)
				if decision == Exit && code == 0 {
					code = 1
					cancel()
				}
			}
		}

		result <- code
	}()

	return result
}
