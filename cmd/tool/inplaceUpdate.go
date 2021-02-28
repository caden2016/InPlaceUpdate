package main

import (
	"fmt"
	"InPlaceUpdate/cmd/tool/app"
	//"math/rand"
	"os"
)

func main() {
	//rand.Seed(time.Now().UnixNano())

	command := app.NewKubeCommand()

	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}