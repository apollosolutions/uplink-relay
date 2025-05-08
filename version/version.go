package version

import (
	"fmt"
)

var (
	Version = "dev"
)

func BuildVersion() string {
	return fmt.Sprintf("%s", Version)
}
