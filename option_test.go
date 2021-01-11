package appmain

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestErrorStrategy_Continue(t *testing.T) {
	var errTCs []TaskContext

	app := New(ErrorStrategy(func(tc TaskContext) Decision {
		errTCs = append(errTCs, tc)
		return Continue
	}))

	var count int
	main1 := app.AddMainTask("", func(ctx context.Context) error {
		count++
		return errors.New("aaa")
	})
	main2 := app.AddMainTask("", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		count++
		return errors.New("aaa")
	})
	app.AddMainTask("", func(ctx context.Context) error {
		count++
		return nil
	})

	app.Run()

	equal(t, errTCs, []TaskContext{main1, main2})
	equal(t, count, 3)
}

func TestErrorStrategy_Exit(t *testing.T) {
	var errTCs []TaskContext

	app := New(ErrorStrategy(func(tc TaskContext) Decision {
		errTCs = append(errTCs, tc)
		return Exit
	}))

	var count int
	main1 := app.AddMainTask("", func(ctx context.Context) error {
		count++
		return errors.New("aaa")
	})
	main2 := app.AddMainTask("", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1000 * time.Millisecond):
		}
		count++
		return errors.New("aaa")
	})
	app.AddMainTask("", func(ctx context.Context) error {
		count++
		return nil
	})

	app.Run()

	equal(t, errTCs, []TaskContext{main1, main2})
	equal(t, count, 2)
}
