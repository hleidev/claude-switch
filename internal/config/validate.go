package config

import "regexp"

// envKeyPattern matches a valid POSIX environment variable name. Restricting env
// keys to this set is what keeps shellenv's generated `export KEY=...` lines safe
// to eval: a key containing whitespace or shell metacharacters would otherwise
// break out of the assignment.
var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidEnvKey reports whether k is a safe environment variable name.
func ValidEnvKey(k string) bool {
	return envKeyPattern.MatchString(k)
}
