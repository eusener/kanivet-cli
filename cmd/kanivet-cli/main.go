package main

import (
	"fmt"
	"os"

	"github.com/eusener/kanivet-cli/internal/platform"
)

func main() {
	p, err := platform.Detect()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%+v\n", p)
}
