package main

import "log"

func debugLog(enabled *bool, format string, v ...interface{}) {
	if *enabled {
		log.Printf(format, v...)
	}
}
