package main

import (
	"github.com/sirupsen/logrus"

	"github.com/bluemir/watcher/cmd"
)

func main() {
	if err := cmd.Run(); err != nil {
		logrus.Fatalf("%+v", err)
	}
}
