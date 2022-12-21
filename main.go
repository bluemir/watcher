package main

import (
	"os"

	"github.com/sirupsen/logrus"

	"github.com/bluemir/watcher/cmd"
)

func main() {
	if err := cmd.Run(); err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
}
