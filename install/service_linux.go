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

package install

import (
	"io/ioutil"
	"os/exec"
	"github.com/pufferpanel/pufferd/logging"
	"syscall"
	"github.com/pufferpanel/pufferd/utils"
)

const SYSTEMD = `
[Unit]
Description=pufferd daemon service

[Service]
Type=simple
WorkingDirectory=/srv/pufferd
ExecStart=/srv/pufferd/pufferd --config ${configpath}
User=pufferd
Group=pufferd

[Install]
WantedBy=multi-user.target
`

func InstallService(configPath string) {
	cmd := exec.Command("useradd", "--system", "--home", "/var/lib/pufferd","--user-group", "pufferd")

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if status.ExitStatus() != 9 {
					logging.Error("Error creating pufferd user", err)
					return
				}
			} else {
				logging.Error("Error creating pufferd user", err)
				return
			}
		} else {
			logging.Error("Error creating pufferd user", err)
			return
		}
	}

	updates := make(map[string]interface{})
	updates["configpath"] = configPath

	err = ioutil.WriteFile("/etc/systemd/system/pufferd.service", []byte(utils.ReplaceTokens(SYSTEMD, updates)), 0664)
	if err != nil {
		logging.Error("Cannot write systemd file, will not install service", err)
		return
	}

	cmd = exec.Command("systemctl", "enable", "pufferd")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logging.Error("Error enabling pufferd service", err)
		logging.Error(string(output))
		return
	}
	logging.Info(string(output))

	logging.Info("Service may be started with: systemctl start pufferd")
}
