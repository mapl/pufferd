package programs

import (
	"encoding/json"
	"errors"
	"github.com/pufferpanel/apufferi/common"
	"github.com/pufferpanel/apufferi/config"
	"github.com/pufferpanel/apufferi/logging"
	"github.com/pufferpanel/pufferd/environments"
	"github.com/pufferpanel/pufferd/programs/operations"
	"io/ioutil"
	"os"
)

type ServerJson struct {
	ProgramData ProgramData `json:"pufferd"`
}

type ProgramData struct {
	Data            map[string]DataObject  `json:"data"`
	Display         string                 `json:"display"`
	EnvironmentData map[string]interface{} `json:"environment"`
	InstallData     InstallSection         `json:"install"`
	Type            string                 `json:"type"`
	Identifier      string                 `json:"id"`
	RunData         RunObject              `json:"run"`
	Template        string                 `json:"template"`

	Environment  environments.Environment `json:"-"`
	CrashCounter int                      `json:"-"`
}

type DataObject struct {
	Description  string      `json:"desc"`
	Display      string      `json:"display"`
	Internal     bool        `json:"internal"`
	Required     bool        `json:"required"`
	Value        interface{} `json:"value"`
	UserEditable bool        `json:"userEdit"`
}

type RunObject struct {
	Arguments               []string                 `json:"arguments"`
	Program                 string                   `json:"program"`
	Stop                    string                   `json:"stop"`
	Enabled                 bool                     `json:"enabled"`
	AutoStart               bool                     `json:"autostart"`
	AutoRestartFromCrash    bool                     `json:"autorecover"`
	AutoRestartFromGraceful bool                     `json:"autorestart"`
	Pre                     []map[string]interface{} `json:"pre"`
	Post                    []map[string]interface{} `json:"post"`
	StopCode                int                      `json:"stopCode,omitempty"`
	EnvironmentVariables    map[string]string        `json:"environmentVars"`
}

type InstallSection struct {
	Operations []map[string]interface{} `json:"commands"`
}

func (p ProgramData) DataToMap() map[string]interface{} {
	var result = make(map[string]interface{}, len(p.Data))

	for k, v := range p.Data {
		result[k] = v.Value
	}

	return result
}

func CreateProgram() ProgramData {
	return ProgramData{
		RunData: RunObject{
			Enabled:                 true,
			AutoStart:               false,
			AutoRestartFromCrash:    false,
			AutoRestartFromGraceful: false,
			Pre:                  make([]map[string]interface{}, 0),
			Post:                 make([]map[string]interface{}, 0),
			EnvironmentVariables: make(map[string]string, 0),
		},
		Type:    "standard",
		Data:    make(map[string]DataObject, 0),
		Display: "Unknown server",
		InstallData: InstallSection{
			Operations: make([]map[string]interface{}, 0),
		},
	}
}

//Starts the program.
//This includes starting the environment if it is not running.
func (p *ProgramData) Start() (err error) {
	if !p.IsEnabled() {
		logging.Errorf("Server %s is not enabled, cannot start", p.Id())
		return errors.New("server not enabled")
	}
	logging.Debugf("Starting server %s", p.Id())
	p.Environment.DisplayToConsole("Starting server\n")
	data := make(map[string]interface{})
	for k, v := range p.Data {
		data[k] = v.Value
	}

	process := operations.GenerateProcess(p.RunData.Pre, p.Environment, p.DataToMap(), p.RunData.EnvironmentVariables)
	err = process.Run()
	if err != nil {
		p.Environment.DisplayToConsole("Error running pre execute, check daemon logs\n")
		return
	}

	err = p.Environment.ExecuteAsync(p.RunData.Program, common.ReplaceTokensInArr(p.RunData.Arguments, data), common.ReplaceTokensInMap(p.RunData.EnvironmentVariables, data), p.afterExit)
	if err != nil {
		logging.Error("Error starting server", err)
		p.Environment.DisplayToConsole("Failed to start server\n")
	}

	return
}

//Stops the program.
//This will also stop the environment it is ran in.
func (p *ProgramData) Stop() (err error) {
	logging.Debugf("Stopping server %s", p.Id())
	if p.RunData.StopCode != 0 {
		err = p.Environment.SendCode(p.RunData.StopCode)
	} else {
		err = p.Environment.ExecuteInMainProcess(p.RunData.Stop)
	}
	if err != nil {
		p.Environment.DisplayToConsole("Failed to stop server\n")
	} else {
		p.Environment.DisplayToConsole("Server stopped\n")
	}
	return
}

//Kills the program.
//This will also stop the environment it is ran in.
func (p *ProgramData) Kill() (err error) {
	logging.Debugf("Killing server %s", p.Id())
	err = p.Environment.Kill()
	if err != nil {
		p.Environment.DisplayToConsole("Failed to kill server\n")
	} else {
		p.Environment.DisplayToConsole("Server killed\n")
	}
	return
}

//Creates any files needed for the program.
//This includes creating the environment.
func (p *ProgramData) Create() (err error) {
	logging.Debugf("Creating server %s", p.Id())
	p.Environment.DisplayToConsole("Allocating server\n")
	err = p.Environment.Create()
	p.Environment.DisplayToConsole("Server allocated\n")
	p.Environment.DisplayToConsole("Ready to be installed\n")
	return
}

//Destroys the server.
//This will delete the server, environment, and any files related to it.
func (p *ProgramData) Destroy() (err error) {
	logging.Debugf("Destroying server %s", p.Id())
	err = p.Environment.Delete()
	return
}

func (p *ProgramData) Install() (err error) {
	if !p.IsEnabled() {
		logging.Errorf("Server %s is not enabled, cannot install", p.Id())
		return errors.New("server not enabled")
	}

	logging.Debugf("Installing server %s", p.Id())
	running, err := p.IsRunning()
	if err != nil {
		logging.Error("Error stopping server to install: ", err)
		p.Environment.DisplayToConsole("Error stopping server\n")
		return
	}

	if running {
		err = p.Stop()
	}

	if err != nil {
		logging.Error("Error stopping server to install: ", err)
		p.Environment.DisplayToConsole("Error stopping server\n")
		return
	}

	p.Environment.DisplayToConsole("Installing server\n")

	os.MkdirAll(p.Environment.GetRootDirectory(), 0755)

	var process operations.OperationProcess

	if len(p.InstallData.Operations) == 0 && p.Template != "" {
		logging.Debugf("Server %s has no defined install data, using template", p.Id())
		templateData, err := ioutil.ReadFile(common.JoinPath(TemplateFolder, p.Template+".json"))
		if err != nil {
			logging.Error("Error reading template for "+p.Template, err)
			p.Environment.DisplayToConsole("Error running installer, check daemon logs\n")
			return err
		}

		templateJson := ServerJson{}
		err = json.Unmarshal(templateData, &templateJson)
		if err != nil {
			logging.Error("Malformed json for program "+p.Template, err)
			p.Environment.DisplayToConsole("Error running installer, check daemon logs\n")
			return err
		}

		process = operations.GenerateProcess(templateJson.ProgramData.InstallData.Operations, p.GetEnvironment(), p.DataToMap(), p.RunData.EnvironmentVariables)

	} else {
		logging.Debugf("Server %s has defined install data", p.Id())
		process = operations.GenerateProcess(p.InstallData.Operations, p.GetEnvironment(), p.DataToMap(), p.RunData.EnvironmentVariables)
	}

	err = process.Run()
	if err != nil {
		p.Environment.DisplayToConsole("Error running installer, check daemon logs\n")
	} else {
		p.Environment.DisplayToConsole("Server installed\n")
	}
	return
}

//Determines if the server is running.
func (p *ProgramData) IsRunning() (isRunning bool, err error) {
	isRunning, err = p.Environment.IsRunning()
	return
}

//Sends a command to the process
//If the program supports input, this will send the arguments to that.
func (p *ProgramData) Execute(command string) (err error) {
	err = p.Environment.ExecuteInMainProcess(command)
	return
}

func (p *ProgramData) SetEnabled(isEnabled bool) (err error) {
	p.RunData.Enabled = isEnabled
	return
}

func (p *ProgramData) IsEnabled() (isEnabled bool) {
	isEnabled = p.RunData.Enabled
	return
}

func (p *ProgramData) SetEnvironment(environment environments.Environment) (err error) {
	p.Environment = environment
	return
}

func (p *ProgramData) Id() string {
	return p.Identifier
}

func (p *ProgramData) GetEnvironment() environments.Environment {
	return p.Environment
}

func (p *ProgramData) SetAutoStart(isAutoStart bool) (err error) {
	p.RunData.AutoStart = isAutoStart
	return
}

func (p *ProgramData) IsAutoStart() (isAutoStart bool) {
	isAutoStart = p.RunData.AutoStart
	return
}

func (p *ProgramData) Save(file string) (err error) {
	logging.Debugf("Saving server %s", p.Id())

	endResult := ServerJson{}
	endResult.ProgramData = *p

	data, err := json.MarshalIndent(endResult, "", "  ")
	if err != nil {
		return
	}

	err = ioutil.WriteFile(file, data, 0664)
	return
}

func (p *ProgramData) Edit(data map[string]interface{}) (err error) {
	for k, v := range data {
		if v == nil || v == "" {
			delete(p.Data, k)
		}

		var elem DataObject

		if _, ok := p.Data[k]; ok {
			elem = p.Data[k]
		} else {
			elem = DataObject{}
		}
		elem.Value = v

		p.Data[k] = elem
	}
	err = Save(p.Id())
	return
}

func (p *ProgramData) GetData() map[string]DataObject {
	return p.Data
}

func (p *ProgramData) GetNetwork() string {
	data := p.GetData()
	ip := "0.0.0.0"
	port := "0"

	if ipData, ok := data["ip"]; ok {
		ip = ipData.Value.(string)
	}

	if portData, ok := data["port"]; ok {
		port = portData.Value.(string)
	}

	return ip + ":" + port
}

func (p *ProgramData) CopyFrom(s *ProgramData) {
	p.Data = s.Data
	p.RunData = s.RunData
	p.Display = s.Display
	p.EnvironmentData = s.EnvironmentData
	p.InstallData = s.InstallData
	p.Type = s.Type
	p.Template = s.Template
}

func (p *ProgramData) afterExit(graceful bool) {
	if graceful {
		p.CrashCounter = 0
	}

	mapping := p.DataToMap()
	mapping["success"] = graceful

	processes := operations.GenerateProcess(p.RunData.Post, p.Environment, mapping, p.RunData.EnvironmentVariables)

	p.Environment.DisplayToConsole("Running post-execution steps\n")
	logging.Debugf("Running post execution steps: %s", p.Id())

	err := processes.Run()
	if err != nil {
		logging.Error("Error running post processing")
		p.Environment.DisplayToConsole("Error executing post steps\n")
		return
	}

	if !p.RunData.AutoRestartFromCrash && !p.RunData.AutoRestartFromGraceful {
		return
	}

	if graceful && p.RunData.AutoRestartFromGraceful {
		StartViaService(p)
	} else if !graceful && p.RunData.AutoRestartFromCrash && p.CrashCounter < config.GetIntOrDefault("crashlimit", 3) {
		p.CrashCounter++
		StartViaService(p)
	}
}
