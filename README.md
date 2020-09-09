# WorkScheduler

## Schedule commands to be executed when certain conditions (e.g. internet connectivity) are met and they don't impact the user (not running on battery, no user interacting, etc.).

It consists of a daemon mode that waits for good conditions to execute the scheduled commands and a command line client to add commands for scheduled later execution.  
Currently there is only a check for when external power is available again.

It is written in Go and currently I am mainly learning the language with this project, so the code is not really pretty at the moment :)