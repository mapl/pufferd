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

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/braintree/manners"
	"github.com/gin-gonic/gin"
	"github.com/pufferpanel/apufferi/config"
	"github.com/pufferpanel/apufferi/logging"
	"github.com/pufferpanel/pufferd/commands"
	"github.com/pufferpanel/pufferd/data"
	"github.com/pufferpanel/pufferd/data/templates"
	"github.com/pufferpanel/pufferd/install"
	"github.com/pufferpanel/pufferd/migration"
	"github.com/pufferpanel/pufferd/programs"
	"github.com/pufferpanel/pufferd/routing"
	"github.com/pufferpanel/pufferd/sftp"
	"github.com/pufferpanel/pufferd/shutdown"
	"net/http"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
)

var (
	VERSION      = "nightly"
	MAJORVERSION = "nightly"
	GITHASH      = "unknown"
)

var runService = true
var configPath string

func main() {
	var loggingLevel string
	var authRoot string
	var authToken string
	var runInstaller bool
	var version bool
	var license bool
	var regenerate bool
	var migrate bool
	var shutdownPid int
	var runDaemon bool
	var reloadPid int
	flag.StringVar(&loggingLevel, "logging", "INFO", "Lowest logging level to display")
	flag.StringVar(&authRoot, "auth", "", "Base URL to the authorization server")
	flag.StringVar(&authToken, "token", "", "Authorization token")
	flag.BoolVar(&runInstaller, "install", false, "If installing instead of running")
	flag.BoolVar(&version, "version", false, "Get the version")
	flag.BoolVar(&license, "license", false, "View license")
	flag.BoolVar(&regenerate, "regenerate", false, "Regenerate pufferd templates")
	flag.BoolVar(&migrate, "migrate", false, "Migrate Scales data to pufferd")
	flag.StringVar(&configPath, "config", "config.json", "Path to pufferd config.json")
	flag.IntVar(&shutdownPid, "shutdown", 0, "PID to shut down")
	flag.IntVar(&reloadPid, "reload", 0, "PID to shut down")
	flag.BoolVar(&runDaemon, "run", false, "Runs the daemon")
	flag.Parse()

	versionString := fmt.Sprintf("pufferd %s (%s)", VERSION, GITHASH)

	if shutdownPid != 0 {
		logging.Info("Shutting down")
		commands.Shutdown(shutdownPid)
	}

	if reloadPid != 0 {
		logging.Info("Reloading")
		commands.Reload(reloadPid)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) && !runInstaller && !version && reloadPid == 0 && shutdownPid == 0 {
		if _, err := os.Stat("/etc/pufferd/config.json"); err == nil {
			logging.Info("No config passed, defaulting to /etc/pufferd/config.json")
			configPath = "/etc/pufferd/config.json"
		} else {
			logging.Error("Cannot find a config file!")
			return
		}
	}

	if version {
		os.Stdout.WriteString(versionString + "\r\n")
	}

	if license {
		os.Stdout.WriteString(data.LICENSE + "\r\n")
	}

	if regenerate {
		commands.Regenerate(configPath)
	}

	if migrate {
		config.Load(configPath)
		migration.MigrateFromScales()
	}

	if license || version || regenerate || migrate || shutdownPid != 0 || reloadPid != 0 {
		return
	}

	config.Load(configPath)

	logging.SetLevelByString(loggingLevel)
	var defaultLogFolder = "logs"
	if runtime.GOOS == "linux" {
		defaultLogFolder = "/var/log/pufferd"
	}
	var logPath = config.GetStringOrDefault("logPath", defaultLogFolder)
	logging.SetLogFolder(logPath)
	logging.Init()
	gin.SetMode(gin.ReleaseMode)

	logging.Info(versionString)
	logging.Info("Logging set to " + loggingLevel)

	if runInstaller {
		install.Install(configPath, authRoot, authToken)
	}

	if runInstaller || !runDaemon {
		return
	}

	programs.Initialize()

	if _, err := os.Stat(programs.TemplateFolder); os.IsNotExist(err) {
		logging.Info("No template directory found, creating")
		err = os.MkdirAll(programs.TemplateFolder, 0755)
		if err != nil {
			logging.Error("Error creating template folder", err)
		}

	}
	if files, _ := ioutil.ReadDir(programs.TemplateFolder); len(files) == 0 {
		logging.Info("Templates being copied to " + programs.TemplateFolder)
		templates.CopyTemplates()
	}

	if _, err := os.Stat(programs.ServerFolder); os.IsNotExist(err) {
		logging.Info("No server directory found, creating")
		os.MkdirAll(programs.ServerFolder, 0755)
	}

	//check if there's an update
	go CheckForUpdate()

	programs.LoadFromFolder()

	programs.InitService()

	for _, element := range programs.GetAll() {
		if element.IsEnabled() {
			element.GetEnvironment().DisplayToConsole("Daemon has been started\n")
			if element.IsAutoStart() {
				logging.Info("Queued server " + element.Id())
				element.GetEnvironment().DisplayToConsole("Server has been queued to start\n")
				programs.StartViaService(element)
			}
		}
	}

	CreateHook()

	for runService {
		runServices()
	}

	shutdown.Shutdown()
}

func runServices() {
	r := routing.ConfigureWeb()

	useHttps := false

	dataFolder := config.GetStringOrDefault("datafolder", "data")
	httpsPem := filepath.Join(dataFolder, "https.pem")
	httpsKey := filepath.Join(dataFolder, "https.key")

	if _, err := os.Stat(httpsPem); os.IsNotExist(err) {
		logging.Warn("No HTTPS.PEM found in data folder, will use http instead")
	} else if _, err := os.Stat(httpsKey); os.IsNotExist(err) {
		logging.Warn("No HTTPS.KEY found in data folder, will use http instead")
	} else {
		useHttps = true
	}

	sftp.Run()

	web := config.GetStringOrDefault("web", config.GetStringOrDefault("webhost", "0.0.0.0")+":"+config.GetStringOrDefault("webport", "5656"))

	logging.Infof("Starting web access on %s", web)
	var err error
	if useHttps {
		err = manners.ListenAndServeTLS(web, httpsPem, httpsKey, r)
	} else {
		err = manners.ListenAndServe(web, r)
	}
	if err != nil {
		logging.Error("Error starting web service", err)
	}
}

func CreateHook() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.Signal(15), syscall.Signal(1))
	go func() {
		defer func() {
			if err := recover(); err != nil {
				logging.Errorf("Error: %+v\n%s", err, debug.Stack())
			}
		}()

		var sig os.Signal

		for sig != syscall.Signal(15) {
			sig = <-c
			switch sig {
			case syscall.Signal(1):
				manners.Close()
				sftp.Stop()
				config.Load(configPath)
			}
		}

		runService = false
		shutdown.CompleteShutdown()
	}()
}

func CheckForUpdate() {
	if config.GetBoolOrDefault("update-check", true) {
		url := "https://dl.pufferpanel.com/pufferd/" + MAJORVERSION + "/version.txt"
		logging.Debug("Checking for updates using " + url)
		resp, err := http.Get(url)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		onlineVersion := strings.TrimSpace(string(body))
		if string(onlineVersion) != GITHASH {
			logging.Infof("DL server reports a different hash than this version, an update may be available")
			logging.Infof("Installed: %s", GITHASH)
			logging.Infof("Online: %s", onlineVersion)
		}
	}
}
