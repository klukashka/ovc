//go:build slice6_measurement

package api_test

import "flag"

var slice6Smoke = flag.Bool("slice6-smoke", false, "Exercise measurement fixtures with small datasets; never report performance acceptance")

func measurementCount(full int, smoke int) int {
	if *slice6Smoke {
		return smoke
	}
	return full
}
