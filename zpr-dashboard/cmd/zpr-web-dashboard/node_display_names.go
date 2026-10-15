package main

import (
	"os"
	"strings"

	"neboagency.com/zpr-dashborad/internal/controlservice/inventory"
)

func readNodeDisplayNames(filePath string) (map[string]string, error) {
	return inventory.ReadNodeDisplayNames(filePath)
}

func populateNodeDisplayNames(out *snapshot) {
	inventory.PopulateNodeDisplayNames(out, strings.TrimSpace(os.Getenv("ZPR_NODE_DISPLAY_NAMES_FILE")))
}
