package main

import (
	"fmt"
	"os"

	"github.com/akhmed9505/comment-tree/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
