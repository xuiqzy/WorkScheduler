package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/distatus/battery"

	// consider toml, json or json with comments like json5 or disabling some dangerous yaml features
	// in the parser like anchors that make xml bomb like attacks possible
	// easy to edit (json - no spaces to be cautious of) important?
	// use whats best as data store, as its not supposed to be looked at a lot
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

func main() {

	var runningInDaemonMode bool

	fmt.Println("passed arguments:", os.Args)

	var commandToExecuteAbsolutePath string
	numberOfCommandLineArguments := len(os.Args)
	// first argument is the path to this program itself, so more than 1 argument means user passed some command as argument
	if numberOfCommandLineArguments >= 2 {

		runningInDaemonMode = false
		// we just add the command to the command store and exit

		// careful, user supplied input!
		commandToExecuteAbsolutePath = os.Args[1]
		// todo: check if full path, don't use path lookup as standard to prevent path injection attacks

		var commandArguments []string
		if numberOfCommandLineArguments >= 3 {
			commandArguments = os.Args[2:]
		} else {
			fmt.Println("Info: No arguments specified for the command to run.")
		}
		err := addCommandToStoredCommands(CommandWithArguments{UUID: uuid.New(), AbsolutePath: commandToExecuteAbsolutePath, CommandArguments: commandArguments})
		if err != nil {
			fmt.Println("Error when adding command to command store for later execution:", err)
		} else {
			fmt.Println("Successfully added command to command store. It will be executed later.")
		}

	} else {
		fmt.Println("No command to run specified, running in daemon mode and executing stored commands when appropriate")
		runningInDaemonMode = true
	}

	if runningInDaemonMode {
		for {

			waitUntilPluggedIn()

			// run all commands that were scheduled:
			commandStore, err := readAndParseCommandStore()
			if err != nil {
				fmt.Println("Error when reading command store:", err)
				fmt.Println("Trying again later, when also plugged into external power.")
				sleepForSeconds(5)
				continue
			}

			commandStoreMap := make(map[uuid.UUID]CommandWithArguments)
			for _, commandWithArguments := range commandStore.Commands {
				commandStoreMap[commandWithArguments.UUID] = commandWithArguments
			}
			// todo use concurrency here
			for currentUUID, currentCommand := range commandStoreMap {
				if isDeviceRunningOnBatteryPower() {
					// go to beginning of outer for loop where we wait for computer to be plugged in
					break
				}
				// TODO:
				// parallelize this but take care that command is always removed from newly read or
				// synchronized / used by all goroutines commandStore and not the initial one at the
				// moment of the goroutine starting.....
				runCommandWithArgumentsAndHandleErrors(currentCommand)

				commandStore = removeCommandFromCommandStore(commandStoreMap, currentUUID)
				marshalAndWriteCommandStore(commandStore)

				// todo remove command from list and save it back to storage somewhere
			}
			if len(commandStore.Commands) == 0 {
				fmt.Println("No commands left to execute, waiting for new commands to be added")
			}
			// TODO: check here if new commands are there and don't sleep then or
			// just keep scheduling commands to execute before until command store (on disk) is empty

			secondsToSleep := 5
			fmt.Println("Sleeping for", secondsToSleep, "seconds...")
			sleepForSeconds(secondsToSleep)
		}
		//runCommandAndHandleErrors(commandToExecuteAbsolutePath, commandArguments...)
		// ignore potentially returned err for now when command not successful, already logged in function itself

	}
	//Println("(Test) Running only on battery power: ", isDeviceRunningOnBatteryPowerOnly())

}

func removeCommandFromCommandStore(commandStoreMap map[uuid.UUID]CommandWithArguments, uuidOfCommandToRemove uuid.UUID) CommandStore {

	_, ok := commandStoreMap[uuidOfCommandToRemove]
	if ok {
		delete(commandStoreMap, uuidOfCommandToRemove)
	} else {
		fmt.Println("this shouldn't happen...")
	}

	commands := make([]CommandWithArguments, 0, len(commandStoreMap))
	for _, currentCommand := range commandStoreMap {
		commands = append(commands, currentCommand)
	}
	commandStore := CommandStore{Commands: commands}
	return commandStore

	//commandStore.Commands[index] = commandStore.Commands[len(commandStore.Commands)-1]
	// We do not need to put s[i] at the end, as it will be discarded anyway
	//commandStore.Commands = commandStore.Commands[:len(commandStore.Commands)-1]
	//return commandStore
}

var pathToCommandStoreFile = "./commandStore.yaml"

// CommandStore contains all commands to be executed some time
type CommandStore struct {
	Commands []CommandWithArguments
}

// CommandWithArguments is a full path to a command with all arguments to it, in whole representing what should be executed
type CommandWithArguments struct {
	UUID             uuid.UUID
	AbsolutePath     string
	CommandArguments []string `yaml:",flow"`
}

func readAndParseCommandStore() (CommandStore, error) {
	return readAndParseCommandStoreFromFile(pathToCommandStoreFile)
}

func readAndParseCommandStoreFromFile(pathToCommandStoreFile string) (CommandStore, error) {
	commandStore := CommandStore{}
	yamlFile, readingError := ioutil.ReadFile(pathToCommandStoreFile)
	if readingError != nil {
		fmt.Printf("error when reading command store from disk: %v\n", readingError)
		return commandStore, readingError
	}
	unmarshalError := yaml.Unmarshal(yamlFile, &commandStore)
	if unmarshalError != nil {
		fmt.Printf("error when trying to unmarshal command store content: %v\n", unmarshalError)
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
		fmt.Printf("error when trying to marshal command store content: %v\n", marshalError)
		return marshalError
	}

	// write / overwrite to configured data store file with newly added data
	// mode is 0 for regular file and only readable and writeable by own user
	writeError := ioutil.WriteFile(pathToCommandStoreFile, marshalledData, 0600)
	if writeError != nil {
		fmt.Printf("error when trying to write marshaled command store content: %v\n", writeError)
		return writeError
	}
	return nil
}

func addCommandToStoredCommands(commandWithArguments CommandWithArguments) error {

	fmt.Println("Adding the following command to the command store for later execution:", commandWithArguments)

	commandStore, readError := readAndParseCommandStore()

	if readError != nil {
		return readError
	}
	commandStore.Commands = append(commandStore.Commands, commandWithArguments)

	// !!! this could lead to data loss if main daemon removes items from command store while this
	// part reads und later writes to the command store

	writeError := marshalAndWriteCommandStore(commandStore)

	return writeError
}

func waitUntilPluggedIn() {
	for isDeviceRunningOnBatteryPower() {
		numberOfSecondsToWait := 2
		fmt.Println("Running only on battery power, waiting for", numberOfSecondsToWait, "seconds")
		sleepForSeconds(numberOfSecondsToWait)
	}

	fmt.Println("External power connected")

}

func sleepForSeconds(numberOfSecondsToSleep int) {
	time.Sleep(multiplyDuration(numberOfSecondsToSleep, time.Second))
}

/*
Hide semantically invalid duration math or seemingly unnecessary/illogical cast behind a function
see https://stackoverflow.com/questions/17573190/how-to-multiply-duration-by-integer
*/
func multiplyDuration(factor int, duration time.Duration) time.Duration {
	// converts duration to nanoseconds because multiplication only works with same types in go
	// uses the new nanoseconds value to construct the new duration
	return time.Duration(int64(factor) * int64(duration))
}

func runCommandWithArgumentsAndHandleErrors(commandWithArguments CommandWithArguments) error {
	return runRawCommandAndHandleErrors(commandWithArguments.AbsolutePath, commandWithArguments.CommandArguments...)
}

func runRawCommandAndHandleErrors(commandToRun string, ArgumentsForCommandToRun ...string) error {

	fmt.Println("Executing command", "`"+commandToRun+"`", "with arguments: ", ArgumentsForCommandToRun)

	// this works without an absolute path at the moment but maybe we should change that
	// to prevent some PATH injection attacks
	command := exec.Command(commandToRun, ArgumentsForCommandToRun...)
	standardOutAndError, err := command.CombinedOutput()

	if err != nil {
		fmt.Println("Error executing command and/or reading standard out and standard error of it:", err)
	}

	// TODO log to system log or sth, just run as systemd unit
	fmt.Println("======== Standard out and error of command ========")
	fmt.Print(string(standardOutAndError))

	fmt.Println("======== End of standard out and error ========")

	return err
}

func isDeviceRunningOnBatteryPower() bool {

	batteries, err := battery.GetAll()
	if err != nil {
		fmt.Println("Could not get battery info!")
		// return true as fallback to not do work when potentially running on battery
		return true
	}

	for _, currentBattery := range batteries {
		if currentBattery.State == battery.Discharging {
			// if at least one battrey is discharing, the external power (if present)
			// is not enough to charge the laptop as a whole and it is
			// losing charge on at least one battery
			// This is a case where we consider it running on battery power
			return true
		}
	}
	// no battery is discharging so every battery is either charging or full and power is plugged in
	// if device has no battery at all, it is running on external power
	return false

}
