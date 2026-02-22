package player

import "os/exec"

func OpenIINA(target string) error {
	return exec.Command("open", "-a", "IINA", target).Start()
}
