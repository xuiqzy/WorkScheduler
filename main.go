package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/distatus/battery"
	"github.com/google/uuid"
)

func main() {
	fmt.Println("Started WorkScheduler :)")

	numberOfCommandLineArguments := len(os.Args)
	// first argument is the path to this program itself, so more than 1 argument means user passed some command as argument
	if numberOfCommandLineArguments >= 2 {
		// we just add the command to the command store and exit

		// careful, user supplied input!
		commandToExecuteAbsolutePath := os.Args[1]
		// todo: check if full absolute path, don't use path lookup as standard to prevent path injection attacks

		var commandArguments []string
		if numberOfCommandLineArguments >= 3 {
			// careful, user supplied input!
			commandArguments = os.Args[2:]
		} else {
			fmt.Println("Info: No arguments specified for the command to run.")
		}
		fmt.Println("Adding the following command to the command store for later execution...")
		fmt.Printf("Absolute path: %q\n", commandToExecuteAbsolutePath)

		fmt.Print("Argument list: ")
		for _, currentArgument := range commandArguments {
			fmt.Printf("%q ", currentArgument)
		}
		fmt.Println()

		newUUID, err := addCommandToCommandStore(commandToExecuteAbsolutePath, commandArguments, 999999999*time.Second, uuid.New().String())
		if err != nil {
			fmt.Println("Error when adding command to command store for later execution:", err)
		} else {
			fmt.Println("Successfully added with uuid:", newUUID)
			fmt.Println("It will be executed later.")
		}

	} else {
		runDaemonMode()
	}

}

func runDaemonMode() {
	fmt.Println("No command to add to scheduled commands specified, running in daemon mode and executing stored commands when appropriate")

	parseAllConfigFiles()

	for {

		waitUntilPowerPluggedIn()

		fmt.Println("Checking command store for commands to be run...")
		commandStore, err := readAndParseCommandStore()
		if err != nil {
			fmt.Println("Error when reading command store:", err)
			fmt.Println("Trying again later (only when also plugged into external power).")
			amountSeconds := 5
			fmt.Println("Sleeping for", amountSeconds, "seconds...")
			fmt.Println()
			sleepForSeconds(amountSeconds)
			continue
		}

		startedExecutingAtLeastOneCommand := false
		for _, currentCommand := range commandStore.Commands {

			// ignore error, just use the returned true as fallback, we will check again later
			runningOnBattery, _ := isDeviceRunningOnBatteryPower()
			if runningOnBattery {
				// don't schedule another command when running on battery
				// go to beginning of outer for loop where we wait for computer to be plugged in again
				break
			}

			if !shouldCommandBeRun(currentCommand) {
				continue
			}

			startedExecutingAtLeastOneCommand = true

			// run current command asynchronously

			// make function with argument here so each coroutine has its own copy of the
			// respective current command and does not share one reference
			go func(commandToRun CommandWithArguments) {
				runRawCommandAndHandleErrors(commandToRun)
			}(currentCommand)

		}

		if !startedExecutingAtLeastOneCommand {
			fmt.Println("No command waiting to be run -> did not start new execution of a command.")
		}

		// we ran all commands asynchronously (if any), wait a bit before checking again
		// for new commands to be scheduled (even if we are still plugged into power)
		secondsToSleep := 10
		fmt.Println("Sleeping for", secondsToSleep, "seconds...")
		fmt.Println()
		sleepForSeconds(secondsToSleep)

	}
}

func shouldCommandBeRun(command CommandWithArguments) bool {
	if command.State == CommandRunning {
		return false
	}

	// TODO: handle command failed state

	// command was never run before
	if command.LastRun.IsZero() {
		return true
	}

	if time.Now().Sub(command.LastRun) > command.DurationBetweenRuns {
		return true
	}
	return false
}

func runRawCommandAndHandleErrors(commandToRun CommandWithArguments) error {

	absolutePath := commandToRun.AbsolutePath
	argumentList := commandToRun.CommandArguments
	uuidOfCommand := commandToRun.UUID

	changeStateToRunningError := changeStateOfCommand(uuidOfCommand, CommandRunning)
	if changeStateToRunningError != nil {
		fmt.Println("Error when changing state of command", commandToRun, "error: ", changeStateToRunningError)
	}

	fmt.Println("Executing command `"+absolutePath+"` with arguments: ", argumentList, "and uuid:", uuidOfCommand)

	// todo: this works without an absolute path at the moment but maybe we should change that
	// to prevent some PATH injection attacks
	command := exec.Command(absolutePath, argumentList...)
	standardOutAndError, err := command.CombinedOutput()

	var stateChangeError error = nil
	if err != nil {
		fmt.Println("Error executing command and/or reading standard out and standard error of it:", err)
		stateChangeError = changeStateOfCommand(uuidOfCommand, CommandFailed)
	} else {
		fmt.Println("Successfully executed command `"+absolutePath+"` with arguments: ", argumentList, "and uuid:", uuidOfCommand)
		stateChangeError = changeStateOfCommand(uuidOfCommand, CommandSuccessful)
	}

	if stateChangeError != nil {
		fmt.Println("Error when changing state of command", commandToRun, "error: ", stateChangeError)
	}

	// TODO log to system log or sth, just run as systemd unit
	fmt.Println()
	fmt.Println("======== Standard out and error of command", commandToRun, "========")
	fmt.Print(string(standardOutAndError))
	fmt.Println("======== End of standard out and error ========")
	fmt.Println()

	return err
}

func waitUntilPowerPluggedIn() {

	// ignore error, just use the returned true as fallback, we will just check again later
	// if it is running on battery
	for runningOnBattery, _ := isDeviceRunningOnBatteryPower(); runningOnBattery; runningOnBattery, _ = isDeviceRunningOnBatteryPower() {
		numberOfSecondsToWait := 10
		fmt.Println("Running only on battery power, waiting for", numberOfSecondsToWait, "seconds")
		sleepForSeconds(numberOfSecondsToWait)
	}

	fmt.Println("External power is currently connected")

}

func isDeviceRunningOnBatteryPower() (bool, error) {

	// This often returns an error shortly after being plugged in, but is fine a few seconds later
	// and returns the correct value then
	batteries, err := battery.GetAll()
	if err != nil {
		// handle error here, rest of program is happy with true as fallback for now and does
		// not use the returned error currently
		fmt.Println("Could not get battery info! Error:", err)
		// return true as fallback to not start commands when potentially running on battery
		return true, err
	}

	// check if there is a battery that is discharging to determine if running on battery or AC:
	for _, currentBattery := range batteries {
		if currentBattery.State == battery.Discharging {
			// If at least one battery is discharing, the external power (if present)
			// is not enough to charge the laptop as a whole and it is
			// losing charge on at least one battery.
			// This is a case where we consider it running on battery power.
			return true, nil
		}
	}

	// when device has a battery:
	// no battery is discharging so every battery is either charging or full
	// -> device not runing on battery

	// when device has no battery:
	// no battery that is discharging was found, because there are no batteries
	// -> device not running on battery
	return false, nil

}