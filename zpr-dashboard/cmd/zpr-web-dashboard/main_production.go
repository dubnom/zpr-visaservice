//go:build production

package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
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
		if err := requireProductionControlRoomConfiguration(); err != nil {
			return err
		}
		return runControlRoom(listen)
	case "control-service":
		return runControlService()
	case "simulator":
		return errors.New("Simulator mode is unavailable in the production build; use the Simulator build")
	default:
		return fmt.Errorf("run mode %q is unavailable in the production build; use control-room or control-service", mode)
	}
}

func requireProductionControlRoomConfiguration() error {
	required := []string{
		"ZPR_CONTROL_ROOM_CERT_FILE",
		"ZPR_CONTROL_ROOM_KEY_FILE",
		"ZPR_CONTROL_ROOM_ORIGIN",
		"ZPR_OPERATOR_OIDC_CONFIG_FILE",
		"ZPR_OPERATOR_OIDC_SECRET_FILE",
		"ZPR_CONTROL_ROOM_ORGANIZATION_ID",
	}
	var missing []string
	for _, name := range required {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		return fmt.Errorf("production control-room mode requires explicit operator configuration: %s", strings.Join(missing, ", "))
	}
	return nil
}
