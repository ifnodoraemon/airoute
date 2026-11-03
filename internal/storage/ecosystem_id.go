package storage

import (
	"fmt"
	"regexp"
)

var ecosystemIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

// ValidateEcosystemID keeps IDs usable as route segments, ZIP roots and client
// install directory names, without path traversal or executable shell syntax.
func ValidateEcosystemID(id string) error {
	if !ecosystemIDPattern.MatchString(id) {
		return fmt.Errorf("ID 需以字母或数字开头，仅含字母、数字、短横线或下划线，长度不超过 128")
	}
	return nil
}
