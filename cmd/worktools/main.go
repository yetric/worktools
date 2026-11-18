package main

import (
	"github.com/yetric/worktools/internal/cli"
	"os"
)

func main() { os.Exit(cli.Run(os.Args)) }
