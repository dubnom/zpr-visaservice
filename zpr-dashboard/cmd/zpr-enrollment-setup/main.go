//go:build linux || darwin || windows

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"neboagency.com/zpr-dashborad/internal/enrollment"
)

func main() {
	path := flag.String("config", "", "Required trusted local setup JSON configuration file")
	userState := flag.Bool("user-state", false, "Use logged-in user's private home state; configuration must omit state_directory")
	openBrowser := flag.Bool("open-browser", false, "Open the private setup URL in the local default browser")
	requiredProtection := flag.String("require-key-protection", "", "Require an exact configured key-protection mode; never fall back")
	flag.Parse()
	if *path == "" || flag.NArg() != 0 {
		log.Fatal("usage: zpr-enrollment-setup -config /path/to/setup.json")
	}
	var config enrollment.SetupConfig
	var err error
	if *userState {
		config, err = enrollment.LoadUserSetupConfig(*path)
	} else {
		config, err = enrollment.LoadSetupConfig(*path)
	}
	if err != nil {
		log.Fatal(err)
	}
	if *requiredProtection != "" && config.KeyProtection != *requiredProtection {
		log.Fatal("Setup configuration does not match the launcher's required key protection; no identity or request was created")
	}
	setup, err := enrollment.NewSetupServer(config)
	if err != nil {
		log.Fatal(err)
	}
	defer setup.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Println("Development software-key setup only. Open this private, one-hour URL in this machine's browser:")
	fmt.Println(setup.URL())
	if *openBrowser {
		if err := openSetupBrowser(setup.URL()); err != nil {
			log.Printf("Could not open the default browser; use the private URL above: %v", err)
		}
	}
	fmt.Println("Do not share the URL. Keep this command running; press Ctrl-C when finished.")
	if err := setup.Run(ctx); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
