/*
 Copyright 2016 Padduck, LLC

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 	http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package environments

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pufferpanel/apufferi/cache"
	"github.com/pufferpanel/apufferi/config"
	"github.com/pufferpanel/pufferd/utils"
	"io"
	"os"
	"sync"
)

type Environment interface {
	//Executes a command within the environment.
	Execute(cmd string, args []string, env map[string]string, callback func(graceful bool)) (stdOut []byte, err error)

	//Executes a command within the environment and immediately return
	ExecuteAsync(cmd string, args []string, env map[string]string, callback func(graceful bool)) (err error)

	//Sends a string to the StdIn of the main program process
	ExecuteInMainProcess(cmd string) (err error)

	//Kills the main process, but leaves the environment running.
	Kill() (err error)

	//Creates the environment setting needed to run programs.
	Create() (err error)

	//Deletes the environment.
	Delete() (err error)

	Update() (err error)

	IsRunning() (isRunning bool, err error)

	WaitForMainProcess() (err error)

	WaitForMainProcessFor(timeout int) (err error)

	GetRootDirectory() string

	GetConsole() (console []string, epoch int64)

	GetConsoleFrom(time int64) (console []string, epoch int64)

	AddListener(ws *websocket.Conn)

	GetStats() (map[string]interface{}, error)

	DisplayToConsole(msg string, data ...interface{})

	SendCode(code int) error
}

type BaseEnvironment struct {
	Environment
	RootDirectory      string                 `json:"-"`
	ConsoleBuffer      cache.Cache            `json:"-"`
	WSManager          utils.WebSocketManager `json:"-"`
	wait               sync.WaitGroup
	Type               string `json:"type"`
	executeAsync       func(cmd string, args []string, env map[string]string, callback func(graceful bool)) (err error)
	waitForMainProcess func() (err error)
}

func (e *BaseEnvironment) Execute(cmd string, args []string, env map[string]string, callback func(graceful bool)) (stdOut []byte, err error) {
	stdOut = make([]byte, 0)
	err = e.ExecuteAsync(cmd, args, env, callback)
	if err != nil {
		return
	}
	err = e.WaitForMainProcess()
	return
}

func (e *BaseEnvironment) WaitForMainProcess() (err error) {
	return e.waitForMainProcess()
}

func (e *BaseEnvironment) ExecuteAsync(cmd string, args []string, env map[string]string, callback func(graceful bool)) (err error) {
	return e.executeAsync(cmd, args, env, callback)
}

func (e *BaseEnvironment) GetRootDirectory() string {
	return e.RootDirectory
}

func (e *BaseEnvironment) GetConsole() (console []string, epoch int64) {
	console, epoch = e.ConsoleBuffer.Read()
	return
}

func (e *BaseEnvironment) GetConsoleFrom(time int64) (console []string, epoch int64) {
	console, epoch = e.ConsoleBuffer.ReadFrom(time)
	return
}

func (e *BaseEnvironment) AddListener(ws *websocket.Conn) {
	e.WSManager.Register(ws)
}

func (e *BaseEnvironment) DisplayToConsole(msg string, data ...interface{}) {
	if len(data) == 0 {
		fmt.Fprint(e.ConsoleBuffer, msg)
		fmt.Fprint(e.WSManager, msg)
	} else {
		fmt.Fprintf(e.ConsoleBuffer, msg, data...)
		fmt.Fprintf(e.WSManager, msg, data...)
	}
}

func (e *BaseEnvironment) Update() error {
	return nil
}

func (e *BaseEnvironment) Delete() (err error) {
	err = os.RemoveAll(e.RootDirectory)
	return
}

func (e *BaseEnvironment) createWrapper() io.Writer {
	if config.GetBoolOrDefault("forward", false) {
		return io.MultiWriter(os.Stdout, e.ConsoleBuffer, e.WSManager)
	}
	return io.MultiWriter(e.ConsoleBuffer, e.WSManager)
}
