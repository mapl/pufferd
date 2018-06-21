package shutdown

import (
	"github.com/braintree/manners"
	"github.com/pufferpanel/apufferi/logging"
	"github.com/pufferpanel/pufferd/programs"
	"os"
	"runtime/debug"
	"sync"
)

func CompleteShutdown() {
	logging.Warn("Interrupt received, stopping servers")
	wg := Shutdown()
	wg.Wait()
	logging.Info("All servers stopped")
	os.Exit(0)
}

func Shutdown() *sync.WaitGroup {
	defer func() {
		if err := recover(); err != nil {
			logging.Errorf("Error: %+v\n%s", err, debug.Stack())
		}
	}()
	wg := sync.WaitGroup{}
	programs.ShutdownService()
	manners.Close()
	prgs := programs.GetAll()
	wg.Add(len(prgs))
	for _, element := range prgs {
		go func(e programs.Program) {
			defer wg.Done()
			defer func() {
				if err := recover(); err != nil {
					logging.Errorf("Error: %+v\n%s", err, debug.Stack())
				}
			}()
			logging.Warn("Stopping program " + e.Id())
			running, err := e.IsRunning()
			if err != nil {
				logging.Error("Error stopping server "+e.Id(), err)
				return
			}
			if !running {
				return
			}
			err = e.Stop()
			if err != nil {
				logging.Error("Error stopping server "+e.Id(), err)
				return
			}
			err = e.GetEnvironment().WaitForMainProcess()
			if err != nil {
				logging.Error("Error stopping server "+e.Id(), err)
				return
			}
			logging.Warn("Stopped program " + e.Id())
		}(element)
	}
	return &wg
}
