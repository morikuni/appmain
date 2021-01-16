# go-appmain

[![test](https://github.com/morikuni/go-appmain/workflows/test/badge.svg?branch=main)](https://github.com/morikuni/go-appmain/actions?query=branch%3Amain)
[![Go Reference](https://pkg.go.dev/badge/github.com/morikuni/go-appmain.svg)](https://pkg.go.dev/github.com/morikuni/go-appmain)
[![Go Report Card](https://goreportcard.com/badge/github.com/morikuni/go-appmain)](https://goreportcard.com/report/github.com/morikuni/go-appmain)
[![codecov](https://codecov.io/gh/morikuni/go-appmain/branch/main/graph/badge.svg)](https://codecov.io/gh/morikuni/go-appmain)

`appmain` simplifies you main function.

## Features

- The graceful shutdown by signals.
- Easy management task dependencies.
- Common operations with interceptors.
- Flexible error strategy.

## Usage

Try it on [The Go Playground](https://play.golang.org/p/dNkNQDNDB73).

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/morikuni/go-appmain"
	_ "github.com/proullon/ramsql/driver"
)

func main() {
	app := appmain.New(
		appmain.ErrorStrategy(func(tc appmain.TaskContext) appmain.Decision {
			fmt.Println(tc.Name()+":", tc.Err())
			return appmain.Continue
		}),
		appmain.DefaultTaskOptions(
			appmain.Interceptor(func(ctx context.Context, tc appmain.TaskContext, task appmain.Task) error {
				fmt.Println("enter:", tc.Name())
				defer fmt.Println("exit:", tc.Name())
				return task(ctx)
			}),
		),
	)

	var db *sql.DB
	app.AddInitTask("initialize db", func(ctx context.Context) error {
		var err error
		db, err = sql.Open("ramsql", "test")
		return err
	})

	server := &http.Server{
		Addr: ":8080",
	}
	httpServerTask := app.AddMainTask("start http server", func(ctx context.Context) error {
		return server.ListenAndServe()
	})

	app.AddCleanupTask("stop http server", func(ctx context.Context) error {
		fmt.Println("stop http server")
		return server.Shutdown(ctx)
	})
	app.AddCleanupTask("close db", func(ctx context.Context) error {
		return db.Close()
	}, appmain.RunAfter(httpServerTask)) // since db should stop after http server, RunAfter is available.

	time.AfterFunc(time.Second, func() { app.SendSignal(os.Interrupt) })

	os.Exit(app.Run())
}
```

- Main tasks run after all init tasks completed.
- Cleanup tasks always run even if signal received during executing init tasks.
