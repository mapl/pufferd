package commands

import (
	"github.com/pufferpanel/apufferi/config"
	"github.com/pufferpanel/apufferi/logging"
	"github.com/pufferpanel/pufferd/data/templates"
	"github.com/pufferpanel/pufferd/programs"
	"os"
)

func Regenerate(configPath string) {
	config.Load(configPath)
	programs.Initialize()

	if _, err := os.Stat(programs.TemplateFolder); os.IsNotExist(err) {
		logging.Info("No template directory found, creating")
		err = os.MkdirAll(programs.TemplateFolder, 0755)
		if err != nil {
			logging.Error("Error creating template folder", err)
		}
	}
	// Overwrite existing templates
	templates.CopyTemplates()
	logging.Info("Templates regenerated")
}
