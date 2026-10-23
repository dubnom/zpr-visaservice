//go:build !production

package main

import "embed"

//go:embed static/*
var staticFiles embed.FS

const productionAssetsOnly = false
