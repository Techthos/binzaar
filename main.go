package main

import (
	"fmt"
	"os"

	"techthos.net/binzaar/cmd"
)

func main() {
	if err := cmd.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, appName+":", err)
		os.Exit(1)
	}
}

const appName = "binzaar"
