package environments

import (
	"github.com/pufferpanel/pufferd/logging"
	"github.com/pufferpanel/pufferd/utils"
)

func LoadEnvironment(environmentType, folder, id string, environmentSection map[string]interface{}) Environment {
	serverRoot := utils.JoinPath(folder, id)
	rootDirectory := utils.GetStringOrDefault(environmentSection, "root", serverRoot)
	switch environmentType {
	case "tty":
		logging.Debugf("Loading server as tty")
		return &tty{RootDirectory: rootDirectory, ConsoleBuffer: utils.CreateCache(), WSManager: utils.CreateWSManager()}
	case "docker":
		logging.Debugf("Loading server as docker")
		netBindings := make([]string, 0)
		image := utils.GetStringOrDefault(environmentSection, "image", "pufferpanel:default")
		return &docker{ContainerId: id, RootDirectory: rootDirectory, ConsoleBuffer: utils.CreateCache(), WSManager: utils.CreateWSManager(), NetworkBindings: netBindings, DockerImage: image}
	default:
		logging.Debugf("Loading server as standard")
		return &standard{RootDirectory: rootDirectory, ConsoleBuffer: utils.CreateCache(), WSManager: utils.CreateWSManager()}
	}
}
