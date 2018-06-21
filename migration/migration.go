package migration

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"bytes"
	"encoding/json"
	"os"

	"github.com/pufferpanel/apufferi/common"
	"github.com/pufferpanel/apufferi/config"
	"github.com/pufferpanel/apufferi/logging"
	"github.com/pufferpanel/pufferd/data/templates"
	"github.com/pufferpanel/pufferd/programs"
)

const Scales = "/srv/scales/data"

func MigrateFromScales() {
	programs.Initialize()
	templates.CopyTemplates()
	os.MkdirAll(programs.ServerFolder, 0755)

	name := "legacyminecraft"
	data := templates.Vanilla
	jsonData := []byte(data)
	var prettyJson bytes.Buffer
	json.Indent(&prettyJson, jsonData, "", "  ")
	err := ioutil.WriteFile(common.JoinPath(programs.TemplateFolder, name+".json"), prettyJson.Bytes(), 0664)
	if err != nil {
		logging.Error("Error writing template "+name, err)
	}

	programFiles, err := ioutil.ReadDir(Scales)
	if err != nil {
		logging.Critical("Error reading from old Scales folder", err)
		return
	}
	for _, element := range programFiles {
		if element.IsDir() {
			continue
		}
		id := strings.TrimSuffix(element.Name(), filepath.Ext(element.Name()))
		logging.Infof("Attempting to migrate %s", id)
		data, err := ioutil.ReadFile(common.JoinPath(Scales, element.Name()))
		if err != nil {
			logging.Error("Error read server config", err)
			continue
		}
		scales := scalesServer{}
		err = json.Unmarshal(data, &scales)
		if err != nil {
			logging.Error("Error read server config", err)
			continue
		}
		oldPath := common.JoinPath("/home", scales.User, "public")
		newPath := common.JoinPath(config.GetStringOrDefault("serverfolder", common.JoinPath("data", "servers")), scales.Name)
		err = os.Rename(oldPath, newPath)
		if err != nil {
			logging.Error("Error moving folder", err)
			continue
		}

		err = filepath.Walk(newPath, func(name string, info os.FileInfo, err error) error {
			if err == nil {
				err = os.Chown(name, os.Getuid(), os.Getgid())
			}
			return err
		})
		if err != nil {
			logging.Error("Error changing owner of folder", err)
			continue
		}
		serverData := make(map[string]interface{})
		serverData["ip"] = scales.Gamehost
		serverData["port"] = scales.Gameport
		if scales.Plugin == "minecraft" {
			scales.Plugin = "legacyminecraft"
			serverData["memory"] = scales.Build.Memory
		} else if scales.Plugin == "srcds" {
			serverData["appid"] = scales.Startup.Variables.Build_Params
			serverData["gametype"] = scales.Startup.Variables.Game
			serverData["map"] = scales.Startup.Variables.Map
		}
		programs.Create(scales.Name, scales.Plugin, serverData)
	}
	os.Remove(common.JoinPath(programs.TemplateFolder, "legacyminecraft.json"))
	logging.Info("Migration complete, please restart pufferd to have it recognize the changes")
}

type scalesServer struct {
	Name     string              `json:"name,omitempty"`
	User     string              `json:"user,omitempty"`
	Build    scalesServerBuild   `json:"build,omitempty"`
	Gameport int                 `json:"gameport,omitempty"`
	Gamehost string              `json:"gamehost,omitempty"`
	Plugin   string              `json:"plugin,omitempty"`
	Startup  scalesServerStartup `json:"startup,omitempty"`
}

type scalesServerBuild struct {
	Memory int `json:"memory,omitempty"`
}

type scalesServerStartup struct {
	Variables scalesServerStartupVariables `json:"variables,omitempty"`
}

type scalesServerStartupVariables struct {
	Build_Params string `json:"build_params,omitempty"`
	Game         string `json:"game,omitempty"`
	Map          string `json:"map,omitempty"`
	Players      string `json:"players,omitempty"`
}
