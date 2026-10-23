"""Compare source-contract acceptance results with the immutable triage inventory."""
import collections
import hashlib
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
BASELINE = HERE.parent / "failure-comparison.json"


def cases(report):
    rows = []

    def visit(suite):
        for spec in suite.get("specs", []):
            for test in spec["tests"]:
                result = test["results"][-1] if test["results"] else {}
                rows.append({
                    "id": spec["id"], "project": test["projectName"],
                    "file": spec["file"], "title": spec["title"],
                    "status": test["status"], "expected_status": test["expectedStatus"],
                    "result_status": result.get("status"),
                    "errors": [error.get("message", "") for error in result.get("errors", [])],
                })
        for child in suite.get("suites", []):
            visit(child)

    for suite in report["suites"]:
        visit(suite)
    assert len({row["id"] for row in rows}) == len(rows), "Duplicate test IDs"
    return rows


def run_summary(name):
    path = HERE / f"{name}.json"
    report = json.loads(path.read_text())
    rows = cases(report)
    projects = collections.defaultdict(collections.Counter)
    for row in rows:
        if row["status"] == "expected":
            category = "passed"
        elif row["status"] == "skipped":
            category = "skipped" if row["expected_status"] == "skipped" else "not_run"
        else:
            category = "failed" if row["status"] == "unexpected" else row["status"]
        projects[row["project"]][category] += 1
    return rows, {
        "raw_json": path.name, "sha256": hashlib.sha256(path.read_bytes()).hexdigest(),
        "stats": report["stats"], "total": len(rows),
        "projects": dict(projects),
        "counts": dict(sum(projects.values(), collections.Counter())),
    }


baseline = json.loads(BASELINE.read_text())
targets = {row["id"] for row in baseline["failures"] if row["classification"] == "new_maturing_contract_drift"}
before = {row["id"] for row in baseline["failures"] if row["maturing_status"] == "unexpected"}
assert len(targets) == 24 and len(before) == 120
focused, focused_summary = run_summary("focused")
boundary, boundary_summary = run_summary("boundary-trusted-source")
full, full_summary = run_summary("full")
assert {row["id"] for row in focused} == targets, "Focused selection differs from authorized IDs"
assert all(row["status"] == "expected" for row in focused), "Authorized focused case failed"
assert all(row["status"] == "expected" for row in boundary), "Boundary/trusted-source case failed"
by_id = {row["id"]: row for row in full}
assert before <= by_id.keys(), "Baseline failing test IDs disappeared"
assert len(full) == baseline["runs"]["maturing"]["total"], "Suite discovery changed"
assert all(by_id[test_id]["status"] == "expected" for test_id in targets), "Authorized full-suite case failed"
after = {row["id"] for row in full if row["status"] == "unexpected"}
resolved = before - after
new_failures = after - before
exact = resolved == targets and not new_failures
comparison = {
    "baseline_inventory": "../failure-comparison.json",
    "baseline_inventory_sha256": hashlib.sha256(BASELINE.read_bytes()).hexdigest(),
    "baseline_run": baseline["runs"]["maturing"],
    "focused": focused_summary, "boundary_trusted_source": boundary_summary,
    "full": full_summary,
    "strict_audit": {"command": "python3 zpr-dashboard/scripts/check-control-room-boundary.py", "exit_code": 0, "log": "strict-audit.log"},
    "authorized_ids": sorted(targets), "authorized_passed_in_full": len(targets),
    "expected_remaining_failing_ids": sorted(before - targets),
    "remaining_failing_ids": sorted(after),
    "resolved_failing_ids": sorted(resolved),
    "resolved_outside_authorization": sorted(resolved - targets),
    "new_failing_ids": sorted(new_failures),
    "exactly_authorized_failure_delta": exact,
    "remaining_failures": [row for row in full if row["status"] == "unexpected"],
    "other_status_changes": [by_id[test_id] for test_id in sorted(resolved - targets)],
    "new_failure_inventory_context": [{
        **by_id[test_id],
        "prior_exact_id_inventory": [row for row in baseline["failures"] if row["id"] == test_id],
        "prior_same_title_other_project": [{
            "id": row["id"], "project": row["project"], "classification": row["classification"],
            "baseline_status": row["baseline_status"], "maturing_status": row["maturing_status"],
        } for row in baseline["failures"] if row["title"] == by_id[test_id]["title"] and row["id"] != test_id],
    } for test_id in sorted(new_failures)],
}
(HERE / "comparison.json").write_text(json.dumps(comparison, indent=2) + "\n")
print(json.dumps({key: comparison[key] for key in ["authorized_passed_in_full", "resolved_outside_authorization", "new_failing_ids", "exactly_authorized_failure_delta"]}, indent=2))
print(json.dumps(full_summary["counts"], indent=2))
raise SystemExit(0 if exact else 1)
