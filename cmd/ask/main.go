package main

import (
	"context"
	"os"

	"github.com/KevinLeeNJ/ask/internal/bootstrap"
)

func main() {
	os.Exit(bootstrap.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
