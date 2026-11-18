package main

import (
	"fmt"
	"os"

	"github.com/akhmed9505/comment-tree/internal/config"
)

func main() {
	if _, err := config.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "load configuration: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("CommentTree")
}
