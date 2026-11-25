//go:build !darwin

package loopback

// Windows and Linux already route all of 127.0.0.0/8 to loopback.
const needsSetup = false

func Configure([]string)   {}
func Unconfigure([]string) {}
