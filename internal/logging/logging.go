package logging

import "log"

// Info is a thin helper to keep call sites stable while logging evolves.
func Info(msg string) {
	log.Println(msg)
}
