package main

// Reads configs from user, which contain which command to execute how often

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml"
)

const configFilesDirectory = "./"

// Config contains a command with arguments and how often to execute it
type Config struct {
	AbsolutePath        string
	Arguments           string
	DurationBetweenRuns time.Duration
}

func getConfigFromFile(pathToConfigFile string) (Config, error) {

	var config = Config{}
	tomlData, err := ioutil.ReadFile(pathToConfigFile)
	if err != nil {
		return config, err
	}

	if err := toml.Unmarshal(tomlData, &config); err != nil {
		return config, err
	}

	return config, nil
}

func getConfigFilesToRead() ([]string, error) {
	files, err := ioutil.ReadDir(configFilesDirectory)
	if err != nil {
		return []string{}, err
	}

	fileNames := make([]string, 0)
	for _, file := range files {
		// filepath.Ext handles filesystem paths, path.Ext would be for other paths like URIs
		if filepath.Ext(file.Name()) == ".toml" {
			fileNames = append(fileNames, file.Name())
		}
	}
	return fileNames, err
}

// TODO what to do with entries which were running when program was closed?

func parseAllConfigFiles() {
	fmt.Println("Reading configs...")

	addedCommandNames := make([]string, 0)

	configFileNames, err := getConfigFilesToRead()
	if err != nil {
		fmt.Println("Couldn't determine what individual config files to read: ", err)
		return
	}
	for _, currentConfigFileName := range configFileNames {
		config, err := getConfigFromFile(currentConfigFileName)
		if err != nil {
			fmt.Println("Error when reading config ", currentConfigFileName, " :", err)
			continue
		}

		absolutePath := config.AbsolutePath
		arguments := config.Arguments
		durationBetweenRuns := config.DurationBetweenRuns
		// containing file name without file extension (last dot and following)
		commandName := strings.TrimSuffix(currentConfigFileName, filepath.Ext(currentConfigFileName))

		hasUpdatedCommandInCommandStore, addError := addCommandToCommandStore(absolutePath, []string{arguments}, durationBetweenRuns, commandName)

		if addError != nil {
			fmt.Println("Couldn't add command:", absolutePath, "with arguments ", arguments, "because:", addError)
		} else {
			// check if we updated the details of a command we have already added from another config file
			// in this method and in this startup of the program and not only that of the command store
			// that represents the state of the previous run (which happens every time the program is restarted
			// without a config file change)
			if hasUpdatedCommandInCommandStore && isStringInSlice(currentConfigFileName, addedCommandNames) {
				fmt.Println("Updated/overwrote details of command that was already added from other config file with the same name.",
					"Filenames must be unique! Affected name:", currentConfigFileName)
			}
			addedCommandNames = append(addedCommandNames, commandName)
		}
	}

	err = removeOldCommandsFromCommandStore(addedCommandNames)
	if err != nil {
		fmt.Println("Error when removing old commands from command store, that are not present anymore in any config file. Error: ", err)
	}
}

func removeOldCommandsFromCommandStore(commandNamesToKeep []string) error {

	commandStore, err := readAndParseCommandStore()
	if err != nil {
		return err
	}

	for _, currentCommand := range commandStore.Commands {
		if !isStringInSlice(currentCommand.Name, commandNamesToKeep) {
			removeError := removeCommandFromCommandStoreByName(currentCommand.Name)
			if removeError != nil {
				return removeError
			}
		}
	}
	return nil
}
