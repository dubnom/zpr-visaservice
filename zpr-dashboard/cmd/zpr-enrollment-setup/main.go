//go:build linux || darwin

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
	setup, err := enrollment.NewSetupServer(config)
	if err != nil {
		log.Fatal(err)
	}
	defer setup.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Println("Development software-key setup only. Open this private, one-hour URL in this machine's browser:")
	fmt.Println(setup.URL())
	fmt.Println("Do not share the URL. Keep this command running; press Ctrl-C when finished.")
	if err := setup.Run(ctx); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
