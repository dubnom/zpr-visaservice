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
	path := flag.String("config", "", "Required private BAS JSON configuration file")
	flag.Parse()
	if *path == "" || flag.NArg() != 0 {
		log.Fatal("usage: zpr-enrollment-bas -config /path/to/private-bas.json")
	}
	config, err := enrollment.LoadBASServerConfig(*path)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := enrollment.RunBASServer(ctx, config); err != nil {
		log.Fatal(err)
	}
}
