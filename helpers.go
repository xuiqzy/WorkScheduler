package main

import "time"

func isStringInSlice(valueToCheck string, sliceToSearch []string) bool {
	for _, currentValue := range sliceToSearch {
		if currentValue == valueToCheck {
			return true
		}
	}
	return false
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
