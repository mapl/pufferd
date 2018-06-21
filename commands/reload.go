package commands

import (
	"errors"
	"github.com/pufferpanel/apufferi/logging"
	"os"
	"syscall"
)

func Reload(pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil || proc == nil {
		if err == nil && proc == nil {
			err = errors.New("no process found")
		}
		logging.Error("Error reloading pufferd", err)
		return
	}
	err = proc.Signal(syscall.Signal(1))
	if err != nil {
		logging.Error("Error reloading pufferd", err)
		return
	}
}
