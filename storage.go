package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/google/uuid"

	// support for locking read/write access to a file within and across processes
	"github.com/gofrs/flock"
)

// Note on file locking:
// create file locks in functions and not once, so different goroutines
// create and have different locks
// and thus have to wait for the other lock on the same file being released
// Waiting for other processes also is handled by that, because file lock
// is visible to other processes through OS mechanisms

var pathToCommandStoreFile = "./commandStore.json"

// CommandStore contains all commands to be executed some time
type CommandStore struct {
	Commands []CommandWithArguments
}

// CommandWithArguments is a full path to a command with all arguments to it, in whole representing what should be executed
type CommandWithArguments struct {
	UUID             uuid.UUID
	AbsolutePath     string
	CommandArguments []string
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

	var fileLockOnCommandStoreFile = flock.New(pathToCommandStoreFile)

	// locking for reading, modifying and writing command store
	// todo handle/relay errors when locking
	fileLockOnCommandStoreFile.Lock()
	defer fileLockOnCommandStoreFile.Unlock()

	commandStore, readError := readAndParseCommandStoreAlreadyLocked()

	if readError != nil {
		return readError
	}

	foundCommandToChangeState := false
	// remove command with specified uuid from list
	for index, currentCommand := range commandStore.Commands {
		if currentCommand.UUID == uuidOfCommandToChangeState {
			//fmt.Println("Changing state of command:", currentCommand, "to state", newState)
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

func addCommandToCommandStore(absolutePathToExecutable string, commandArguments []string) (uuid.UUID, error) {

	var fileLockOnCommandStoreFile = flock.New(pathToCommandStoreFile)

	// locking for reading, modifying and writing command store
	fileLockOnCommandStoreFile.Lock()
	defer fileLockOnCommandStoreFile.Unlock()

	commandStore, readError := readAndParseCommandStoreAlreadyLocked()

	if readError != nil {
		// return the Nil uuid (all 0s), because we can't return nil
		// it is not a normally valid uuid
		return uuid.Nil, readError
	}

	uuidOfNewCommand := uuid.New()

	commandWithArguments := CommandWithArguments{
		UUID:             uuidOfNewCommand,
		AbsolutePath:     absolutePathToExecutable,
		CommandArguments: commandArguments,
		State:            CommandWaitingToBeRun}
	commandStore.Commands = append(commandStore.Commands, commandWithArguments)

	writeError := marshalAndWriteCommandStore(commandStore)

	// writeError is nil on success
	return uuidOfNewCommand, writeError
}

func removeCommandFromCommandStore(uuidOfCommandToRemove uuid.UUID) error {

	var fileLockOnCommandStoreFile = flock.New(pathToCommandStoreFile)

	// locking for reading, modifying and writing command store
	fileLockOnCommandStoreFile.Lock()
	defer fileLockOnCommandStoreFile.Unlock()

	commandStore, readError := readAndParseCommandStoreAlreadyLocked()

	if readError != nil {
		return readError
	}

	foundCommandToRemove := false
	// remove command with specified uuid from list
	for index, currentCommand := range commandStore.Commands {
		if currentCommand.UUID == uuidOfCommandToRemove {
			//fmt.Println("Removing command:", currentCommand)
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

	var fileLockOnCommandStoreFile = flock.New(pathToCommandStoreFile)

	if !alreadyLocked {
		fileLockOnCommandStoreFile.Lock()
	}

	commandStore := CommandStore{}
	// empty file is created by this, if it does not exist
	marshalledJSONData, readingError := ioutil.ReadFile(pathToCommandStoreFile)
	if readingError != nil {
		return commandStore, readingError
	}
	if !alreadyLocked {
		fileLockOnCommandStoreFile.Unlock()
	}

	// Empty file is not valid json, so just return empty command store here before trying to unmarshal.
	// A write to the command store will create valid json in the future.
	if len(marshalledJSONData) == 0 {
		return commandStore, readingError
	}

	unmarshalError := json.Unmarshal(marshalledJSONData, &commandStore)
	if unmarshalError != nil {
		return commandStore, unmarshalError
	}

	return commandStore, nil
}

// never called directly from scheduling logic, only through add command and change command functions
// so the command store file is already locked
func marshalAndWriteCommandStore(commandStore CommandStore) error {
	return marshalAndWriteCommandStoreToFile(pathToCommandStoreFile, commandStore)
}

func marshalAndWriteCommandStoreToFile(pathToCommandStoreFile string, commandStore CommandStore) error {

	// prefix new lines with nothing, indent with tabs
	marshalledJSONData, marshalError := json.MarshalIndent(&commandStore, "", "\t")

	if marshalError != nil {
		return marshalError
	}

	// write to / overwrite configured data store file with provided data:

	// file mode is 0 meaning regular file and 600 means only readable and writeable by own user
	// and executable by no one
	// umask gets applied afterwards and might change permissions of created file
	var permissionsForNewFileBeforeUmask os.FileMode = 0600

	// when overwriting file, permissions are not changed
	writeError := ioutil.WriteFile(pathToCommandStoreFile, marshalledJSONData, permissionsForNewFileBeforeUmask)
	if writeError != nil {
		return writeError
	}
	return nil
}
