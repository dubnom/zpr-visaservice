package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func productionAssetManifest(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("../../packaging/control-room/assets.txt")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

func embeddedAssetNames(t *testing.T, root fs.FS) []string {
	t.Helper()
	var names []string
	err := fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			names = append(names, strings.TrimPrefix(name, "static/"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func TestProductionAssetManifest(t *testing.T) {
	manifest := productionAssetManifest(t)
	if !slices.IsSorted(manifest) || len(slices.Compact(slices.Clone(manifest))) != len(manifest) {
		t.Fatal("asset manifest must be sorted and unique")
	}
	if names := embeddedAssetNames(t, controlRoomFiles); !slices.Equal(names, manifest) {
		t.Fatalf("production embed differs from packaging manifest:\ngot %v\nwant %v", names, manifest)
	}
	// Check the compiler's packaged file set, not only what HTTP chooses to serve.
	cmd := exec.Command("go", "list", "-tags", "production", "-json", ".")
	data, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct{ EmbedFiles []string }
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	embeddedManifest, err := os.ReadFile("../../packaging/control-room/embedded-files.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pkg.EmbedFiles, strings.Fields(string(embeddedManifest))) {
		t.Fatalf("production build embeds nonmanifest artifacts: %v", pkg.EmbedFiles)
	}
	if productionAssetsOnly {
		if directoryAssistantSkill != "" || scenarioAssistantSkill != "" || organizationAssistantSkill != "" {
			t.Fatal("production includes Simulator assistant artifacts")
		}
		if names := embeddedAssetNames(t, staticFiles); !slices.Equal(names, manifest) {
			t.Fatalf("production legacy filesystem differs: %v", names)
		}
	}
}

func TestProductionAssetsHTTP(t *testing.T) {
	root, err := fs.Sub(controlRoomFiles, "static")
	if err != nil {
		t.Fatal(err)
	}
	handler := revalidateStatic(http.FileServer(http.FS(root)))
	manifest := productionAssetManifest(t)
	for _, name := range manifest {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/"+name+"?v=boundary", nil))
		if name == "index.html" {
			if response.Code != http.StatusMovedPermanently {
				t.Fatalf("index redirect: %d", response.Code)
			}
		} else if response.Code != http.StatusOK {
			t.Errorf("production asset %s: %d", name, response.Code)
		}
	}
	files, err := os.ReadDir("static")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range files {
		if entry.IsDir() || slices.Contains(manifest, entry.Name()) {
			continue
		}
		for _, method := range []string{"GET", "HEAD"} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(method, "/"+entry.Name()+"?v=boundary", nil))
			if response.Code != http.StatusNotFound {
				t.Errorf("%s Simulator asset %s exposed: %d", method, entry.Name(), response.Code)
			}
		}
	}
	for _, name := range []string{"scenarios.html", "organizations.html", "activity.html", "agents.html", "machine-logs.html", "trusted-source.html", "world.html", "simulator-provisioning.html", "simulator-info.html", "simulator-info.js", "simulator-nav.js", "simulator.css", "simulator-navigation.json", "api/simulator/scenarios", "api/machines", "%73cenarios.html", "scenarios.html/"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/"+name, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("forbidden route %s exposed: %d", name, response.Code)
		}
	}
	// All statically declared production document dependencies must ship.
	references := regexp.MustCompile(`(?:src|href)="/([^"?#]+\.(?:js|css|svg|html))`)
	for _, name := range manifest {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		data, err := fs.ReadFile(root, name)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range references.FindAllStringSubmatch(string(data), -1) {
			if !slices.Contains(manifest, match[1]) {
				t.Errorf("%s references excluded asset %s", name, match[1])
			}
		}
	}
}
