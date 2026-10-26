//go:build production

package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
)

// The production build is the operator entrypoint: it composes only Control
// Room and Control-Service. Simulator, lab and workload modes are compiled out.
func main() {
	mode := flag.String("mode", envOr("ZPR_WEB_MODE", "control-room"), "Run mode: control-room or control-service")
	listen := flag.String("listen", envOr("ZPR_WEB_LISTEN", defaultListen), "Control Room HTTPS listen address")
	flag.Parse()
	if err := runProductionMode(*mode, *listen); err != nil {
		log.Fatal(err)
	}
}

func runProductionMode(mode, listen string) error {
	switch mode {
	case "control-room":
		return runControlRoom(listen)
	case "control-service":
		return runControlService()
	case "simulator":
		return errors.New("Simulator mode is unavailable in the production build; use the Simulator build")
	default:
		return fmt.Errorf("run mode %q is unavailable in the production build; use control-room or control-service", mode)
	}
}
