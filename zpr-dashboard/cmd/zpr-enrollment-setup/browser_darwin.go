package main

import "os/exec"

func openSetupBrowser(url string) error {
	return exec.Command("/usr/bin/open", url).Run()
}
