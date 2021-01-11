package appmain

import (
	"context"
	"testing"
)

func TestRunAfter(t *testing.T) {
	app := New()

	var count int
	createTask := func(want int) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			count++
			if count != want {
				t.Fatalf("want %d got %d", want, count)
			}
			return nil
		}
	}

	init := app.AddInitTask("1", createTask(1))
	app.AddInitTask("2", createTask(2), RunAfter(init))

	main1 := app.AddMainTask("3", createTask(3))
	main2 := app.AddMainTask("4 or 5", func(ctx context.Context) error {
		count++
		return nil
	}, RunAfter(main1))
	main3 := app.AddMainTask("4 or 5", func(ctx context.Context) error {
		count++
		return nil
	}, RunAfter(main1))
	app.AddMainTask("6", createTask(6), RunAfter(main2, main3))

	cleanup := app.AddCleanupTask("7", createTask(7))
	app.AddCleanupTask("8", createTask(8), RunAfter(cleanup))

	app.Run()

	if count != 8 {
		t.Fatalf("want %d got %d", 8, count)
	}
}

func TestIntercept(t *testing.T) {
	app := New()

	var count int
	createInterceptor := func(want int) Interceptor {
		return func(ctx context.Context, taskContext TaskContext, task Task) error {
			count++
			if count != want {
				t.Fatalf("want %d got %d", want, count)
			}
			return task(ctx)
		}
	}

	app.AddMainTask("main", func(ctx context.Context) error {
		return nil
	},
		Intercept(
			createInterceptor(1),
			createInterceptor(2),
			createInterceptor(3),
		),
		Intercept(
			createInterceptor(4),
			createInterceptor(5),
			createInterceptor(6),
		),
		Intercept(
			createInterceptor(7),
			createInterceptor(8),
			createInterceptor(9),
		),
	)

	app.Run()

	if count != 9 {
		t.Fatalf("want %d got %d", 9, count)
	}
}
