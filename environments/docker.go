// +build !windows

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
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/gorilla/websocket"
	"github.com/pufferpanel/pufferd/utils"
	"time"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"io"
	"github.com/pufferpanel/pufferd/config"
)

var dataPath = utils.JoinPath("data", "runtime")

type docker struct {
	ContainerId     string
	NetworkBindings []string
	RootDirectory   string
	WSManager       utils.WebSocketManager
	ConsoleBuffer 	utils.Cache
	DockerImage 	string

	client          *client.Client
	connection      *types.HijackedResponse
	execId          string
	loadedFromCache bool
	proxyWriter 	io.Writer
}

//Executes a command within the environment.
func (d *docker) Execute(cmd string, args []string) (stdOut []byte, err error) {
	err = d.ExecuteAsync(cmd, args)
	if err != nil {
		return
	}
	err = d.WaitForMainProcess()
	return
}

//Executes a command within the environment and immediately return
func (d *docker) ExecuteAsync(cmd string, args []string) (err error) {
	cmdSet := []string{cmd}
	cmdSet = append(cmdSet, args...)
	config := types.ExecConfig{
		Cmd: cmdSet,
		AttachStdin: true,
		AttachStdout: true,
		AttachStderr: true,
	}
	res, err := d.client.ContainerExecCreate(context.Background(), d.ContainerId, config)
	d.execId = res.ID
	os.MkdirAll(dataPath, 0750)
	ioutil.WriteFile(utils.JoinPath(dataPath, d.ContainerId), []byte(d.execId), 0660)
	*d.connection, err = d.client.ContainerExecAttach(context.Background(), d.execId, config)
	d.updateBuffer()
	return
}

//Sends a string to the StdIn of the main program process
func (d *docker) ExecuteInMainProcess(cmd string) (err error) {
	err = d.openConnection()
	if err != nil {
		return
	}

	if d.connection == nil {
		err = errors.New("Process not running")
		return
	}
	_, err = d.connection.Conn.Write([]byte(cmd))
	return
}

//Kills the main process, but leaves the environment running.
func (d *docker) Kill() (err error) {
	err = d.openConnection()
	if err != nil {
		return
	}
	if d.execId == "" {
		return
	}
	err = d.client.ContainerKill(context.Background(), d.execId, "9")
	d.execId = ""
	return
}

//Creates the environment setting needed to run programs.
func (d *docker) Create() (err error) {
	d.openConnection()

	return
}

func (d *docker) Update() (err error) {
	return
}

//Deletes the environment.
func (d *docker) Delete() (err error) {
	opts := types.ContainerRemoveOptions{
		RemoveVolumes: false,
		RemoveLinks: false,
		Force: false,
	}
	err = d.client.ContainerRemove(context.Background(), d.ContainerId, opts)
	return
}

func (d *docker) IsRunning() (isRunning bool) {
	d.openConnection()
	return d.execId != ""
}

func (d *docker) WaitForMainProcess() (err error) {
	err = d.WaitForMainProcessFor(0)
	return
}

func (d *docker) WaitForMainProcessFor(timeout int) (err error) {
	ctx := context.Background()
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout) * time.Second)
		defer cancel()
	}

	_, err = d.client.ContainerWait(ctx, d.ContainerId)
	return
}

func (d *docker) GetRootDirectory() string {
	return d.RootDirectory
}

func (d *docker) GetConsole() ([]string, int64) {
	return d.ConsoleBuffer.Read()
}

func (d *docker) GetConsoleFrom(time int64) ([]string, int64) {
	return d.ConsoleBuffer.ReadFrom(time)
}

func (d *docker) AddListener(ws *websocket.Conn) {
	d.openConnection()
	d.WSManager.Register(ws)
}

func (d *docker) GetStats() (map[string]interface{}, error) {
	d.openConnection()
	data := make(map[string]interface{})
	data["cpu"] = 0
	data["memory"] = 0
	if d.connection != nil {
		_, err := d.client.ContainerStats(context.Background(), d.ContainerId, false)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func (d *docker) openConnection() (err error) {
	if d.client == nil {
		d.client, err = client.NewEnvClient()
	}
	if err != nil {
		return
	}

	if d.loadedFromCache && d.execId == "" {
		return
	}

	//load from cache
	data, err := ioutil.ReadFile(utils.JoinPath(dataPath, d.ContainerId))
	d.loadedFromCache = true
	if err != nil && !os.IsNotExist(err) {
		return
	}
	err = nil

	d.execId = string(data)

	if d.execId == "" {
		return
	}

	res, err := d.client.ContainerExecInspect(context.Background(), d.execId)

	if err != nil {
		return
	}

	if !res.Running {
		d.execId = ""
		return
	}

	opts := types.ContainerAttachOptions{
		Stdin: true,
		Stdout: true,
		Stderr: true,
	}

	*d.connection, err = d.client.ContainerAttach(context.Background(), d.ContainerId, opts)
	d.updateBuffer()
	return
}

func (d *docker) updateBuffer() {
	if d.connection == nil {
		return
	}
	if d.proxyWriter == nil {
		if config.Get("forward") == "true" {
			d.proxyWriter = io.MultiWriter(os.Stdout, d.ConsoleBuffer, d.WSManager)
		} else {
			d.proxyWriter = io.MultiWriter(d.ConsoleBuffer, d.WSManager)
		}
	}

	io.TeeReader(d.connection.Reader, d.proxyWriter)
}