package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The vs-adapter stale-noise wedge is a recorded data-plane defect class (the
// DP-K03 adapter re-dock family): the installed data-plane node loses its
// egress link to the Visa Service fabric address while every container stays
// up; the node log loops with flow_mgmt "no egress link" visa-matching errors
// and noise HandshakeError retries, and BAS relay, visa issuance and
// protected-DNS reconciliation stall until the install is restarted. This
// command is the maintained detection and bounded recovery for that wedge.
// Detection is evidence-first from the node container's recent log window and
// never treats restart transients as the wedge; recovery performs exactly the
// proven remedy — restart the evaluation install unit, then the organization's
// BAS adapter unit — and re-verifies with fresh evidence, failing closed when
// the wedge persists. The root-cause fix belongs to the data plane, not here.

const (
	evaluationAdapterWedgeEgress    = "not found so no egress link"
	evaluationAdapterWedgeHandshake = "HandshakeError"

	// A wedge is a sustained recent loop: the newest signature line must be
	// current, the loop must span a minimum duration, and the container must
	// have been up for the whole detection window so a restarting install's
	// startup transients are never classified as the wedge.
	evaluationAdapterWedgeRecency   = 90 * time.Second
	evaluationAdapterWedgeSustained = 2 * time.Minute
)

type evaluationAdapterLogLine struct {
	At   time.Time
	Text string
}

type evaluationAdapterWedgeVerdict struct {
	Wedged         bool
	Reason         string
	EgressCount    int
	HandshakeCount int
	First          time.Time
	Last           time.Time
	Evidence       []string
}

// evaluateEvaluationAdapterWedge classifies a bounded node log window. It is
// pure so the wedge contract stays test-pinned.
func evaluateEvaluationAdapterWedge(lines []evaluationAdapterLogLine, started, now time.Time, window time.Duration) evaluationAdapterWedgeVerdict {
	verdict := evaluationAdapterWedgeVerdict{}
	var matched []evaluationAdapterLogLine
	for _, line := range lines {
		if line.At.Before(now.Add(-window)) {
			continue
		}
		egress := strings.Contains(line.Text, evaluationAdapterWedgeEgress)
		handshake := strings.Contains(line.Text, evaluationAdapterWedgeHandshake)
		if !egress && !handshake {
			continue
		}
		if egress {
			verdict.EgressCount++
		}
		if handshake {
			verdict.HandshakeCount++
		}
		matched = append(matched, line)
	}
	if len(matched) == 0 {
		verdict.Reason = "no wedge signature in the detection window"
		return verdict
	}
	verdict.First, verdict.Last = matched[0].At, matched[len(matched)-1].At
	for _, line := range matched[:min(3, len(matched))] {
		verdict.Evidence = append(verdict.Evidence, line.At.UTC().Format(time.RFC3339)+" "+line.Text)
	}
	if len(matched) > 3 {
		tail := matched[max(3, len(matched)-3):]
		if len(matched) > 6 {
			verdict.Evidence = append(verdict.Evidence, fmt.Sprintf("… %d further matching lines …", len(matched)-6))
		}
		for _, line := range tail {
			verdict.Evidence = append(verdict.Evidence, line.At.UTC().Format(time.RFC3339)+" "+line.Text)
		}
	}
	switch {
	case started.After(now.Add(-window)):
		verdict.Reason = "container restarted inside the detection window; signature lines are treated as startup transients, not the wedge"
	case now.Sub(verdict.Last) > evaluationAdapterWedgeRecency:
		verdict.Reason = "signature lines are stale; the loop is not current"
	case verdict.Last.Sub(verdict.First) < evaluationAdapterWedgeSustained:
		verdict.Reason = "signature lines do not yet span a sustained loop"
	default:
		verdict.Wedged = true
		verdict.Reason = "sustained current stale-noise wedge signature"
	}
	return verdict
}

// evaluationAdapterRunner executes host commands; tests inject a fake.
type evaluationAdapterRunner func(name string, args ...string) ([]byte, error)

func defaultEvaluationAdapterRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func evaluationAdapterContainerStarted(run evaluationAdapterRunner, container string) (time.Time, error) {
	output, err := run("docker", "inspect", "--format", "{{.State.Running}} {{.State.StartedAt}}", container)
	if err != nil {
		return time.Time{}, fmt.Errorf("inspect node container %s: %v", container, err)
	}
	fields := strings.Fields(strings.TrimSpace(string(output)))
	if len(fields) != 2 || fields[0] != "true" {
		return time.Time{}, fmt.Errorf("node container %s is not running", container)
	}
	started, err := time.Parse(time.RFC3339Nano, fields[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("node container %s start time is unreadable", container)
	}
	return started, nil
}

func evaluationAdapterContainerLogs(run evaluationAdapterRunner, container string, window time.Duration) ([]evaluationAdapterLogLine, error) {
	output, err := run("docker", "logs", "--timestamps", "--since", window.String(), container)
	if err != nil {
		return nil, fmt.Errorf("read node container logs for %s: %v", container, err)
	}
	var lines []evaluationAdapterLogLine
	for _, raw := range strings.Split(string(output), "\n") {
		stamp, text, found := strings.Cut(raw, " ")
		if !found {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			continue
		}
		lines = append(lines, evaluationAdapterLogLine{At: at, Text: text})
	}
	return lines, nil
}

// readEvaluationAdapterInventory performs the minimal read-only inventory
// check this command relies on; it never creates or repairs state.
func readEvaluationAdapterInventory(state string) (evaluationInventory, error) {
	var inventory evaluationInventory
	data, err := os.ReadFile(filepath.Join(state, "evaluation.json"))
	if err != nil || json.Unmarshal(data, &inventory) != nil ||
		inventory.Version != 1 || inventory.Kind != "zpr-evaluation-only" || len(inventory.InstallID) != 24 {
		return inventory, errors.New("incomplete/invalid evaluation inventory; inspect state, never automatically reset it")
	}
	if _, err := hex.DecodeString(inventory.InstallID); err != nil {
		return inventory, errors.New("invalid install identity")
	}
	if !inventory.Config.DataPlane {
		return inventory, errors.New("install has no data-plane container; the stale-noise wedge contract does not apply")
	}
	return inventory, nil
}

func validEvaluationAdapterUnit(unit string) bool {
	if unit == "" || len(unit) > 128 || !strings.HasSuffix(unit, ".service") {
		return false
	}
	for _, r := range unit {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '.' && r != '_' {
			return false
		}
	}
	return true
}

type evaluationAdapterHealthOptions struct {
	state          string
	window         time.Duration
	recover        bool
	installUnit    string
	basAdapterUnit string
	run            evaluationAdapterRunner
	now            func() time.Time
	sleep          func(time.Duration)
	settle         time.Duration
	deadline       time.Duration
}

func runEvaluationAdapterHealth(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-lab-bootstrap -evaluation-adapter-health", flag.ContinueOnError)
	flags.SetOutput(stderr)
	options := evaluationAdapterHealthOptions{
		run: defaultEvaluationAdapterRunner, now: time.Now, sleep: time.Sleep,
		settle: 45 * time.Second, deadline: 4 * time.Minute,
	}
	flags.StringVar(&options.state, "state", "", "existing evaluation install directory (required)")
	flags.DurationVar(&options.window, "window", 5*time.Minute, "node log detection window")
	flags.BoolVar(&options.recover, "recover", false, "when the wedge is detected, run the proven bounded remedy and re-verify")
	flags.StringVar(&options.installUnit, "install-unit", "", "systemd user unit running this evaluation install (required with -recover)")
	flags.StringVar(&options.basAdapterUnit, "bas-adapter-unit", "", "systemd user unit running this organization's BAS adapter (required with -recover)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if options.state == "" || flags.NArg() != 0 ||
		options.window < time.Minute || options.window > time.Hour ||
		(options.recover && (!validEvaluationAdapterUnit(options.installUnit) || !validEvaluationAdapterUnit(options.basAdapterUnit))) ||
		(!options.recover && (options.installUnit != "" || options.basAdapterUnit != "")) {
		fmt.Fprintln(stderr, "usage: zpr-lab-bootstrap -evaluation-adapter-health -state /existing/install [-window 5m] [-recover -install-unit NAME.service -bas-adapter-unit NAME.service]")
		return 2
	}
	fmt.Fprintln(stdout, evaluationBanner)
	code, err := evaluationAdapterHealth(options, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "Adapter health: %v\n", err)
		return 1
	}
	return code
}

func evaluationAdapterHealth(options evaluationAdapterHealthOptions, stdout io.Writer) (int, error) {
	absolute, err := filepath.Abs(options.state)
	if err != nil {
		return 1, errors.New("invalid local state path")
	}
	inventory, err := readEvaluationAdapterInventory(absolute)
	if err != nil {
		return 1, err
	}
	container := "zpr-evaluation-" + inventory.InstallID + "-data-plane"
	verdict, err := evaluationAdapterObserve(options, container)
	if err != nil {
		return 1, err
	}
	writeEvaluationAdapterVerdict(stdout, container, verdict)
	if !verdict.Wedged {
		if options.recover {
			fmt.Fprintln(stdout, "No wedge is detected; refusing a blind restart of a healthy install.")
		}
		return 0, nil
	}
	if !options.recover {
		fmt.Fprintln(stdout, "Proven bounded remedy: restart the evaluation install unit, then the organization's BAS adapter unit (rerun with -recover -install-unit … -bas-adapter-unit …).")
		return 3, nil
	}
	return evaluationAdapterRecover(options, container, stdout)
}

func evaluationAdapterObserve(options evaluationAdapterHealthOptions, container string) (evaluationAdapterWedgeVerdict, error) {
	started, err := evaluationAdapterContainerStarted(options.run, container)
	if err != nil {
		return evaluationAdapterWedgeVerdict{}, err
	}
	lines, err := evaluationAdapterContainerLogs(options.run, container, options.window)
	if err != nil {
		return evaluationAdapterWedgeVerdict{}, err
	}
	return evaluateEvaluationAdapterWedge(lines, started, options.now(), options.window), nil
}

func writeEvaluationAdapterVerdict(stdout io.Writer, container string, verdict evaluationAdapterWedgeVerdict) {
	status := "healthy"
	if verdict.Wedged {
		status = "WEDGED"
	}
	fmt.Fprintf(stdout, "Node container %s: %s (%s; egress-loss lines: %d, handshake-error lines: %d)\n",
		container, status, verdict.Reason, verdict.EgressCount, verdict.HandshakeCount)
	for _, line := range verdict.Evidence {
		fmt.Fprintf(stdout, "  evidence: %s\n", line)
	}
}

// evaluationAdapterRecover performs the proven bounded remedy and re-verifies
// with fresh evidence: the recovered node must run long enough to pass the
// startup-transient horizon with no current signature lines, and the BAS
// adapter unit must come back active. A persistent wedge fails closed.
func evaluationAdapterRecover(options evaluationAdapterHealthOptions, container string, stdout io.Writer) (int, error) {
	fmt.Fprintf(stdout, "Recovering: restarting %s …\n", options.installUnit)
	if output, err := options.run("systemctl", "--user", "restart", options.installUnit); err != nil {
		return 1, fmt.Errorf("restart %s: %v (%s)", options.installUnit, err, strings.TrimSpace(string(output)))
	}
	deadline := options.now().Add(options.deadline)
	for {
		options.sleep(options.settle)
		started, err := evaluationAdapterContainerStarted(options.run, container)
		now := options.now()
		if err == nil && now.Sub(started) >= options.settle {
			lines, err := evaluationAdapterContainerLogs(options.run, container, options.settle)
			if err != nil {
				return 1, err
			}
			current := 0
			for _, line := range lines {
				if now.Sub(line.At) > evaluationAdapterWedgeRecency {
					continue
				}
				if strings.Contains(line.Text, evaluationAdapterWedgeEgress) || strings.Contains(line.Text, evaluationAdapterWedgeHandshake) {
					current++
				}
			}
			if current == 0 {
				fmt.Fprintf(stdout, "Node container %s is running with no current wedge signature.\n", container)
				break
			}
			if now.After(deadline) {
				return 1, fmt.Errorf("wedge signature persists after the install restart (%d current lines); the remedy did not clear it — this is data-plane defect evidence, do not keep restarting", current)
			}
		} else if now.After(deadline) {
			return 1, fmt.Errorf("node container %s did not return to a running settled state after the install restart", container)
		}
	}
	fmt.Fprintf(stdout, "Recovering: restarting %s …\n", options.basAdapterUnit)
	if output, err := options.run("systemctl", "--user", "restart", options.basAdapterUnit); err != nil {
		return 1, fmt.Errorf("restart %s: %v (%s)", options.basAdapterUnit, err, strings.TrimSpace(string(output)))
	}
	basDeadline := options.now().Add(time.Minute)
	for {
		output, err := options.run("systemctl", "--user", "is-active", options.basAdapterUnit)
		if err == nil && strings.TrimSpace(string(output)) == "active" {
			break
		}
		if options.now().After(basDeadline) {
			return 1, fmt.Errorf("BAS adapter unit %s did not return to active after the remedy", options.basAdapterUnit)
		}
		options.sleep(2 * time.Second)
	}
	fmt.Fprintln(stdout, "Recovery complete: install restarted, no current wedge signature, BAS adapter unit active. Verify DNS publication and visas through the ordinary authorized reads; recurrence remains data-plane defect evidence.")
	return 0, nil
}
