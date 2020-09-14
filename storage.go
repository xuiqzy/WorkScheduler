package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync"

	"github.com/google/uuid"

	// consider toml, json or json with comments like json5 or disabling some dangerous yaml features
	// in the parser like anchors that make xml bomb like attacks possible
	// easy to edit (json - no spaces to be cautious of) important?
	// use whats best as data store, as its not supposed to be looked at a lot
	"gopkg.in/yaml.v3"
)

var pathToCommandStoreFile = "./commandStore.yaml"

// TODO: lock with file locking to synchronize with cli adding commands to file
var commandStoreFileLock sync.Mutex

// CommandStore contains all commands to be executed some time
type CommandStore struct {
	Commands []CommandWithArguments
}

// CommandWithArguments is a full path to a command with all arguments to it, in whole representing what should be executed
type CommandWithArguments struct {
	UUID             uuid.UUID
	AbsolutePath     string
	CommandArguments []string `yaml:",flow"`
	State            CommandState
}

// CommandState is one of the states for a command to be in, this will be saved to disk, too
type CommandState string

// states for a command to be in, this will be saved to disk, too
const (
	CommandWaitingToBeRun CommandState = "WaitingToBeRun"
	CommandRunning        CommandState = "Running"
	CommandFailed         CommandState = "Failed"
	CommandSuccessful     CommandState = "Successful"
)

func changeStateOfCommand(uuidOfCommandToChangeState uuid.UUID, newState CommandState) error {

	// locking for reading, modifying and writing command store
	commandStoreFileLock.Lock()
	defer commandStoreFileLock.Unlock()

	commandStore, readError := readAndParseCommandStoreAlreadyLocked()

	if readError != nil {
		return readError
	}

	foundCommandToChangeState := false
	// remove command with specified uuid from list
	for index, currentCommand := range commandStore.Commands {
		if currentCommand.UUID == uuidOfCommandToChangeState {
			fmt.Println("Changing state of command:", currentCommand, "to state", newState)
			commandStore.Commands[index].State = newState
			foundCommandToChangeState = true
			break
		}
	}
	if !foundCommandToChangeState {
		return fmt.Errorf("UUID %v of command to change state to %v not found", uuidOfCommandToChangeState, newState)
	}

	writeError := marshalAndWriteCommandStore(commandStore)

	// is nil on success
	return writeError
}

func addCommandToCommandStore(absolutePathToExecutable string, commandArguments []string) error {

	// locking for reading, modifying and writing command store
	commandStoreFileLock.Lock()
	defer commandStoreFileLock.Unlock()

	commandStore, readError := readAndParseCommandStoreAlreadyLocked()

	if readError != nil {
		return readError
	}
	commandWithArguments := CommandWithArguments{
		UUID:             uuid.New(),
		AbsolutePath:     absolutePathToExecutable,
		CommandArguments: commandArguments,
		State:            CommandWaitingToBeRun}
	commandStore.Commands = append(commandStore.Commands, commandWithArguments)

	// !!! this could lead to data loss if main daemon removes items from command store while this
	// part in another process reads und later writes to the command store

	writeError := marshalAndWriteCommandStore(commandStore)

	// is nil on success
	return writeError
}

func removeCommandFromCommandStore(uuidOfCommandToRemove uuid.UUID) error {

	// locking for reading, modifying and writing command store
	commandStoreFileLock.Lock()
	defer commandStoreFileLock.Unlock()

	commandStore, readError := readAndParseCommandStoreAlreadyLocked()

	if readError != nil {
		return readError
	}

	foundCommandToRemove := false
	// remove command with specified uuid from list
	for index, currentCommand := range commandStore.Commands {
		if currentCommand.UUID == uuidOfCommandToRemove {
			fmt.Println("Removing command:", currentCommand)
			// overwrite current element with last element of list
			commandStore.Commands[index] = commandStore.Commands[len(commandStore.Commands)-1]
			// take list without the last element
			commandStore.Commands = commandStore.Commands[:len(commandStore.Commands)-1]
			foundCommandToRemove = true
			break
		}
	}
	if !foundCommandToRemove {
		return fmt.Errorf("UUID %v of command to remove not found", uuidOfCommandToRemove)
	}

	writeError := marshalAndWriteCommandStore(commandStore)

	// is nil on success
	return writeError
}

// called internally by storage functions when changing state of command, adding or removing commands
// because these actions need to lock the file from before reading until after
// writing their changes so the read function should not take a lock again
func readAndParseCommandStoreAlreadyLocked() (CommandStore, error) {
	return readAndParseCommandStoreFromFile(pathToCommandStoreFile, true)
}

// called from main program to read command store to do something with the
// commands in it
func readAndParseCommandStore() (CommandStore, error) {
	return readAndParseCommandStoreFromFile(pathToCommandStoreFile, false)
}

func readAndParseCommandStoreFromFile(pathToCommandStoreFile string, alreadyLocked bool) (CommandStore, error) {

	if !alreadyLocked {
		commandStoreFileLock.Lock()
	}

	commandStore := CommandStore{}
	yamlFile, readingError := ioutil.ReadFile(pathToCommandStoreFile)
	if readingError != nil {
		return commandStore, readingError
	}
	if !alreadyLocked {
		commandStoreFileLock.Unlock()
	}

	unmarshalError := yaml.Unmarshal(yamlFile, &commandStore)
	if unmarshalError != nil {
		return commandStore, unmarshalError
	}

	return commandStore, nil
}

func marshalAndWriteCommandStore(commandStore CommandStore) error {
	return marshalAndWriteCommandStoreToFile(pathToCommandStoreFile, commandStore)
}

func marshalAndWriteCommandStoreToFile(pathToCommandStoreFile string, commandStore CommandStore) error {

	marshalledData, marshalError := yaml.Marshal(&commandStore)

	if marshalError != nil {
		return marshalError
	}

	// write to / overwrite configured data store file with provided data:

	// file mode is 0 meaning regular file and 600 means only readable and writeable by own user
	// and executable by no one
	// umask gets applied afterwards and might change permissions of created file
	var permissionsForNewFileBeforeUmask os.FileMode = 0600

	// when overwriting file, permissions are not changed
	writeError := ioutil.WriteFile(pathToCommandStoreFile, marshalledData, permissionsForNewFileBeforeUmask)
	if writeError != nil {
		return writeError
	}
	return nil
}
