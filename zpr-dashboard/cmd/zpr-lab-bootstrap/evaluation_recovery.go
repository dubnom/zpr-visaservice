package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/crypto/bcrypt"
)

const recoveryGaps = "Dex static passwords do not support forced change, expiry or single-use login. This is a one-time DISCLOSURE, not an enforced one-time login. Existing JWTs remain signed until expiry; old operator subjects lose Control Room grants on restart. OpenObserve's independent administrator is NOT reset."

func runEvaluationAdminRecovery(state, output string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stdout, evaluationBanner)
	if output == "" {
		terminal, ok := stdout.(*os.File)
		if !ok || !term.IsTerminal(terminal.Fd()) {
			fmt.Fprintln(stderr, "Admin recovery requires a terminal, or -password-file in a private directory; refusing redirected secret output.")
			return 1
		}
	}
	absolute, err := filepath.Abs(state)
	if err != nil {
		fmt.Fprintln(stderr, "Invalid local state path.")
		return 1
	}
	var delivery *os.File
	if output != "" {
		delivery, err = createRecoveryDelivery(output, absolute)
		if err != nil {
			fmt.Fprintln(stderr, "Password delivery requires a NEW file under an owned, real 0700 directory outside install state.")
			return 1
		}
		defer delivery.Close()
	}
	secret, err := resetEvaluationAdmin(absolute, time.Now().UTC())
	if err != nil {
		if delivery != nil {
			_ = os.Remove(delivery.Name())
		}
		fmt.Fprintf(stderr, "Evaluation admin recovery: %v\n", err)
		return 1
	}
	fmt.Fprintln(stderr, recoveryGaps)
	if delivery != nil {
		if _, err := fmt.Fprintln(delivery, secret); err != nil || delivery.Sync() != nil {
			fmt.Fprintln(stderr, "Reset committed but password delivery failed; keep stopped and run a fresh reset with a new output file.")
			return 1
		}
		fmt.Fprintln(stdout, "Reset committed; password delivered only to the new 0600 file. Restart with the original launch flags.")
	} else {
		if _, err := fmt.Fprintf(stdout, "Recovery password (shown once): %s\n", secret); err != nil {
			fmt.Fprintln(stderr, "Reset committed but terminal delivery failed; run a fresh reset.")
			return 1
		}
		fmt.Fprintln(stderr, "Reset committed. Restart with the original launch flags; replace the recovery credential through a fresh local reset after secure handoff.")
	}
	return 0
}

func createRecoveryDelivery(path, state string) (*os.File, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(state, absolute)
	if err != nil || relative == "." || filepath.IsLocal(relative) {
		return nil, errors.New("delivery must be outside state")
	}
	if err := privateRecoveryDirectory(filepath.Dir(absolute)); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		_ = os.Remove(absolute)
		return nil, err
	}
	return file, nil
}

func privateRecoveryDirectory(path string) error {
	real, err := filepath.EvalSymlinks(path)
	if err != nil || real != path {
		return errors.New("directory must be canonical and not symlinked")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 || !recoveryOwned(info) {
		return errors.New("directory must be locally owned and private (0700)")
	}
	return nil
}

func recoveryOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func validateRecoveryState(state string) error {
	if err := privateRecoveryDirectory(state); err != nil {
		return err
	}
	parent, err := os.Stat(filepath.Dir(state))
	if err != nil || parent.Mode().Perm()&0o022 != 0 {
		return errors.New("state parent must not be group/other writable")
	}
	return filepath.WalkDir(state, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return errors.New("cannot inspect private state")
		}
		info, err := entry.Info()
		if err != nil || !recoveryOwned(info) || info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) || info.Mode().Perm()&0o022 != 0 {
			return errors.New("state must be owned, non-writable by others, and contain no symlinks or special files")
		}
		if info.Mode().IsRegular() && info.Sys().(*syscall.Stat_t).Nlink != 1 {
			return errors.New("state must not contain hard-linked files")
		}
		if filepath.Dir(path) == state {
			if info.IsDir() && info.Mode().Perm() != 0o700 {
				return errors.New("top-level state directories must remain private (0700)")
			}
			if !info.IsDir() && info.Mode().Perm() != 0o600 {
				return errors.New("top-level state files must remain private (0600)")
			}
		}
		return nil
	})
}

func lockEvaluation(state string) (*os.File, error) {
	if err := validateRecoveryState(state); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(state, "runtime.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, errors.New("cannot open install lifecycle lock")
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("install lifecycle is busy; stop its foreground bootstrap before recovery or restart")
	}
	return lock, nil
}

func reserveRecoveryPorts(config evaluationConfig) (func(), error) {
	listeners := []net.Listener{}
	closePorts := func() {
		for _, listener := range listeners {
			listener.Close()
		}
	}
	for _, address := range config.listenAddresses() {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			closePorts()
			return nil, errors.New("evaluation ports are occupied; stop all install processes/containers before recovery")
		}
		listeners = append(listeners, listener)
	}
	return closePorts, nil
}

func resetEvaluationAdmin(state string, now time.Time) (string, error) {
	lock, err := lockEvaluation(state)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	data, err := os.ReadFile(filepath.Join(state, "evaluation.json"))
	var inventory evaluationInventory
	if err != nil || json.Unmarshal(data, &inventory) != nil || inventory.Config.validate() != nil {
		return "", errors.New("invalid evaluation state; recovery never creates or reseeds an install")
	}
	inventory, err = readEvaluation(state, inventory.Config)
	if err != nil {
		return "", err
	}
	if inventory.Config.SimulatorPort != 0 {
		simulatorLock, err := lockEvaluationSimulator(state)
		if err != nil {
			return "", err
		}
		defer simulatorLock.Close()
	}
	release, err := reserveRecoveryPorts(inventory.Config)
	if err != nil {
		return "", err
	}
	defer release()
	root, err := os.OpenRoot(state)
	if err != nil {
		return "", errors.New("cannot open protected state")
	}
	defer root.Close()
	dexData, err := root.ReadFile("dex.json")
	var dex struct {
		Passwords []map[string]string `json:"staticPasswords"`
		Clients   []map[string]any    `json:"staticClients"`
	}
	var document map[string]json.RawMessage
	// Subject-link installs add exactly one optional second operator, left unchanged.
	passwords := 1
	if inventory.Config.subjectLinks() {
		passwords = 2
	}
	clients := 1
	if inventory.Config.SimulatorPort != 0 {
		clients = 2
	}
	if err != nil || json.Unmarshal(dexData, &dex) != nil || json.Unmarshal(dexData, &document) != nil || len(dex.Passwords) != passwords || len(dex.Clients) != clients || dex.Passwords[0]["email"] != "admin@evaluation.invalid" || dex.Clients[0]["id"] != "zpr-control-room" ||
		(clients == 2 && dex.Clients[1]["id"] != "zpr-simulator") ||
		(passwords == 2 && dex.Passwords[1]["email"] != evaluationReviewerEmail) {
		return "", errors.New("recovery supports only the bundled administrator (and subject-link reviewer) Dex configuration")
	}
	randomSecret := func() (string, error) {
		value := make([]byte, 32)
		if _, err := rand.Read(value); err != nil {
			return "", errors.New("cannot generate recovery entropy")
		}
		return base64.RawURLEncoding.EncodeToString(value), nil
	}
	password, err := randomSecret()
	if err != nil {
		return "", err
	}
	clientSecret, err := randomSecret()
	if err != nil {
		return "", err
	}
	subject, err := randomSecret()
	if err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", errors.New("cannot hash recovery password")
	}
	dex.Passwords[0]["hash"], dex.Passwords[0]["userID"] = string(hash), subject
	dex.Clients[0]["secret"] = clientSecret
	document["staticPasswords"], _ = json.Marshal(dex.Passwords)
	document["staticClients"], _ = json.Marshal(dex.Clients)
	dexData, err = json.MarshalIndent(document, "", "  ")
	if err != nil {
		return "", errors.New("cannot render recovery configuration")
	}
	files := map[string][]byte{"dex.json": append(dexData, '\n'), "operator-admin.secret": []byte(password), "oidc-room-client.secret": []byte(clientSecret)}
	for name, contents := range files {
		sum := sha256.Sum256(contents)
		inventory.Files[name] = hex.EncodeToString(sum[:])
	}
	files["evaluation.json"], err = json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return "", errors.New("cannot render recovery inventory")
	}
	// The marker makes a crash between atomic replacements fail closed, not reseed.
	if err := writePrivate(root, "admin-recovery.pending", []byte("Interrupted recovery: restore the complete offline backup before restarting.\n")); err != nil {
		return "", errors.New("cannot begin recovery; inspect pending state")
	}
	if err := syncRecoveryDirectory(root); err != nil {
		return "", err
	}
	audit := func(event string) error {
		record, _ := json.Marshal(map[string]any{"event": event, "at": now.Format(time.RFC3339Nano), "install_id": inventory.InstallID, "actor_uid": os.Geteuid(), "account": "admin@evaluation.invalid", "evaluation_only": true, "method": "local-stopped-static-config", "gaps": recoveryGaps})
		file, err := root.OpenFile("admin-recovery.audit.jsonl", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return errors.New("cannot open private recovery audit")
		}
		_, writeErr := file.Write(append(record, '\n'))
		return errors.Join(writeErr, file.Sync(), file.Close())
	}
	if err := audit("admin-recovery-started"); err != nil {
		return "", errors.New("recovery audit failed; keep stopped and restore backup")
	}
	for _, name := range []string{"dex.json", "operator-admin.secret", "oidc-room-client.secret", "evaluation.json"} {
		staged := name + ".admin-recovery-new"
		if err := writePrivate(root, staged, files[name]); err != nil {
			return "", errors.New("recovery staging failed; keep stopped and restore backup")
		}
		if err := root.Rename(staged, name); err != nil {
			return "", errors.New("recovery replacement failed; keep stopped and restore backup")
		}
	}
	// Never leave the previous subject's grants available for manual package startup.
	for _, name := range []string{"operator-oidc.json", "simulator-oidc.json", evaluationDelegationTrust} {
		if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", errors.New("cannot retire old operator grants; keep stopped and restore backup")
		}
	}
	if err := syncRecoveryDirectory(root); err != nil {
		return "", err
	}
	if err := audit("admin-recovery-committed"); err != nil {
		return "", errors.New("recovery audit failed; keep stopped and restore backup")
	}
	if err := root.Remove("admin-recovery.pending"); err != nil {
		return "", errors.New("cannot complete recovery; keep stopped and restore backup")
	}
	if err := syncRecoveryDirectory(root); err != nil {
		return "", err
	}
	return password, nil
}

func syncRecoveryDirectory(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return errors.New("cannot sync private state directory; keep stopped")
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return errors.New("cannot sync recovery state; keep stopped and inspect backup")
	}
	return nil
}
