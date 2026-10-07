package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"neboagency.com/zpr-dashborad/internal/enrollment"
)

func main() {
	path := flag.String("config", "", "Required device enrollment service JSON configuration file")
	flag.Parse()
	if *path == "" || flag.NArg() != 0 {
		log.Fatal("usage: zpr-enrollment-service -config /path/to/device-enrollment.json")
	}
	config, err := enrollment.LoadServerConfig(*path)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := enrollment.RunDeviceServer(ctx, config); err != nil {
		log.Fatal(err)
	}
}
