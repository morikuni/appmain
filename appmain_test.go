package appmain

import (
	"context"
	"os"
	"reflect"
	"syscall"
	"testing"
	"time"
)

func equal(tb testing.TB, a, b interface{}) {
	tb.Helper()

	if !reflect.DeepEqual(a, b) {
		tb.Fatalf("%v != %v", a, b)
	}
}

func TestApp(t *testing.T) {
	for name, tt := range map[string]struct {
		runner       func(app *App) int
		wantCode     int
		wantResult   ResultSet
		wantDuration time.Duration
	}{
		"success": {
			func(app *App) int {
				return app.Run()
			},
			0,
			ResultSet{
				NumInit:        2,
				SuccessInit:    2,
				NumMain:        2,
				SuccessMain:    2,
				NumCleanup:     2,
				SuccessCleanup: 2,
			},
			300 * time.Millisecond,
		},
		"cancel init": {
			func(app *App) int {
				time.AfterFunc(10*time.Millisecond, func() {
					app.SendSignal(os.Interrupt)
				})
				return app.Run()
			},
			0,
			ResultSet{
				NumInit:        2,
				SuccessInit:    1,
				NumMain:        0,
				SuccessMain:    0,
				NumCleanup:     2,
				SuccessCleanup: 2,
			},
			110 * time.Millisecond,
		},
		"cancel main": {
			func(app *App) int {
				time.AfterFunc(110*time.Millisecond, func() {
					app.SendSignal(os.Interrupt)
				})
				return app.Run()
			},
			0,
			ResultSet{
				NumInit:        2,
				SuccessInit:    2,
				NumMain:        2,
				SuccessMain:    1,
				NumCleanup:     2,
				SuccessCleanup: 2,
			},
			210 * time.Millisecond,
		},
		"cancel cleanup": {
			func(app *App) int {
				time.AfterFunc(210*time.Millisecond, func() {
					app.SendSignal(os.Interrupt)
				})
				return app.Run()
			},
			0,
			ResultSet{
				NumInit:        2,
				SuccessInit:    2,
				NumMain:        2,
				SuccessMain:    2,
				NumCleanup:     2,
				SuccessCleanup: 1,
			},
			210 * time.Millisecond,
		},
		"cancel twice init": {
			func(app *App) int {
				time.AfterFunc(10*time.Millisecond, func() {
					app.SendSignal(os.Interrupt)
					time.AfterFunc(10*time.Millisecond, func() {
						app.SendSignal(os.Interrupt)
					})
				})
				return app.Run()
			},
			128 + int(syscall.SIGINT),
			ResultSet{
				NumInit:        2,
				SuccessInit:    1,
				NumMain:        0,
				SuccessMain:    0,
				NumCleanup:     2,
				SuccessCleanup: 1,
			},
			20 * time.Millisecond,
		},
		"cancel twice main": {
			func(app *App) int {
				time.AfterFunc(110*time.Millisecond, func() {
					app.SendSignal(os.Interrupt)
					time.AfterFunc(10*time.Millisecond, func() {
						app.SendSignal(os.Interrupt)
					})
				})
				return app.Run()
			},
			128 + int(syscall.SIGINT),
			ResultSet{
				NumInit:        2,
				SuccessInit:    2,
				NumMain:        2,
				SuccessMain:    1,
				NumCleanup:     2,
				SuccessCleanup: 1,
			},
			120 * time.Millisecond,
		},
		"cancel twice cleanup": {
			func(app *App) int {
				time.AfterFunc(210*time.Millisecond, func() {
					app.SendSignal(os.Interrupt)
					app.SendSignal(os.Interrupt)
				})
				return app.Run()
			},
			128 + int(syscall.SIGINT),
			ResultSet{
				NumInit:        2,
				SuccessInit:    2,
				NumMain:        2,
				SuccessMain:    2,
				NumCleanup:     2,
				SuccessCleanup: 1,
			},
			210 * time.Millisecond,
		},
	} {
		t.Run(name, func(t *testing.T) {
			code, rs, d := runApp(tt.runner)
			equal(t, code, tt.wantCode)
			equal(t, rs, tt.wantResult)
			if d < tt.wantDuration || d > (tt.wantDuration+10*time.Millisecond) {
				t.Fatalf("want %v got %v", tt.wantDuration, d)
			}
		})
	}
}

type ResultSet struct {
	NumInit        int
	SuccessInit    int
	NumMain        int
	SuccessMain    int
	NumCleanup     int
	SuccessCleanup int
}

func runApp(runner func(*App) int) (int, ResultSet, time.Duration) {
	createTask := func(count, success *int, wait bool) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			*count++
			if wait {
				select {
				case <-time.After(100 * time.Millisecond):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			*success++
			return nil
		}
	}

	app := New()
	var (
		nInit int
		sInit int
	)
	app.AddInitTask("init 1", createTask(&nInit, &sInit, false))
	app.AddInitTask("init 2", createTask(&nInit, &sInit, true))

	var (
		nMain int
		sMain int
	)
	app.AddMainTask("main 1", createTask(&nMain, &sMain, false))
	app.AddMainTask("main 2", createTask(&nMain, &sMain, true))

	var (
		nCleanup int
		sCleanup int
	)
	app.AddCleanupTask("cleanup 1", createTask(&nCleanup, &sCleanup, false))
	app.AddCleanupTask("cleanup 2", createTask(&nCleanup, &sCleanup, true))

	start := time.Now()
	code := runner(app)
	d := time.Now().Sub(start)
	return code, ResultSet{
		NumInit:        nInit,
		SuccessInit:    sInit,
		NumMain:        nMain,
		SuccessMain:    sMain,
		NumCleanup:     nCleanup,
		SuccessCleanup: sCleanup,
	}, d
}
