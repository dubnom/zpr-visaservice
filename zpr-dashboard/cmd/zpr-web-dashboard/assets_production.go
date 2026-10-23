//go:build production

package main

// Keep legacy command helpers on the same production-only filesystem.
var staticFiles = controlRoomFiles

const productionAssetsOnly = true
