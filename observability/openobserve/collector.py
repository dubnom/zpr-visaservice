#!/usr/bin/env python3
"""Export Visa Service counters, denials, and process logs to OpenObserve."""

import argparse
import base64
import json
import math
import os
import re
import ssl
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


DEFAULT_RUNTIME = Path("/work/.local-runtime")
POLL_SECONDS = 15
MAX_LOG_BATCH = 200
MAX_PENDING_LOGS = 10000
LOG_LOOKBACK_BYTES = 512 * 1024
SECRET_PATTERNS = (
    re.compile(r"\bzpr_vsapi\.[0-9a-f]{8}\.[A-Za-z0-9_-]+\b", re.IGNORECASE),
    re.compile(r"(?i)(authorization|x-api-key|password|secret|token)\s*[:=]\s*[^\s,;]+"),
)


def read_env_file(path: Path) -> dict[str, str]:
    values = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip()
    return values


def otlp_string_attribute(key: str, value: object) -> dict[str, object]:
    return {"key": key, "value": {"stringValue": str(value)}}


def metric_name(stat_name: str) -> str:
    normalized = re.sub(r"[^a-z0-9]+", "_", stat_name.lower()).strip("_")
    return f"zpr_vs_{normalized}"


def organization_resource(organization_id: str) -> dict[str, object]:
    if not re.fullmatch(r"[a-z][a-z0-9-]{0,79}", organization_id):
        raise ValueError("ZPR organization ID must be a lowercase profile ID")
    return {
        "attributes": [
            otlp_string_attribute("service.name", "zpr-visaservice"),
            otlp_string_attribute("service.instance.id", "local-linux-zpr"),
            otlp_string_attribute("zpr.net", "local-linux-zpr"),
            otlp_string_attribute("zpr.organization.id", organization_id),
        ]
    }


def redact(message: str) -> str:
    for pattern in SECRET_PATTERNS:
        message = pattern.sub("[REDACTED]", message)
    return message


class Collector:
    def __init__(self, runtime: Path) -> None:
        self.runtime = runtime
        self.config = read_env_file(runtime / "observability" / "collector.env")
        self.openobserve_organization = self.required_config("OPENOBSERVE_ORG")
        self.zpr_organization_id = os.environ.get("ZPR_ORGANIZATION_ID", "").strip()
        self.email = self.required_config("OPENOBSERVE_EMAIL")
        self.ingestion_token = (runtime / "observability" / "ingestion.token").read_text(encoding="utf-8").strip()
        self.api_key = (runtime / "admin-read.key").read_text(encoding="utf-8").strip()
        self.admin_url = os.environ.get("ZPR_VS_ADMIN_URL", "https://[fd5a:5052::1]:8182")
        self.openobserve_url = os.environ.get("ZPR_OBSERVABILITY_URL", "http://[fd5a:5052:adda:1::54]:5080")
        self.admin_ca = runtime / "local-admin-cert.pem"
        self.admin_context = ssl.create_default_context(cafile=str(self.admin_ca))
        self.admin_context.check_hostname = False
        self.otlp_auth = base64.b64encode(f"{self.email}:{self.ingestion_token}".encode()).decode()
        self.resource = organization_resource(self.zpr_organization_id)
        self.last_deny_ms = int(time.time() * 1000) - 5 * 60 * 1000
        self.seen_denies: set[tuple[object, ...]] = set()
        self.pending_denies: set[tuple[object, ...]] = set()
        self.pending_logs: list[tuple[dict[str, object], tuple[object, ...] | None, int]] = []
        self.last_uptime: float | None = None
        self.metric_start_ns = time.time_ns()
        self.log_identity: tuple[int, int] | None = None
        self.log_offset = 0
        self.log_path: Path | None = None

    def required_config(self, key: str) -> str:
        value = self.config.get(key, "").strip()
        if not value:
            raise RuntimeError(f"missing {key} in collector.env")
        return value

    def admin_get(self, path: str) -> object:
        request = urllib.request.Request(
            f"{self.admin_url}{path}",
            headers={"X-API-Key": self.api_key, "Accept": "application/json"},
        )
        with urllib.request.urlopen(request, context=self.admin_context, timeout=30) as response:
            return json.loads(response.read())

    def send_otlp(self, signal: str, body: dict[str, object]) -> None:
        organization = urllib.parse.quote(self.openobserve_organization, safe="")
        request = urllib.request.Request(
            f"{self.openobserve_url}/api/{organization}/v1/{signal}",
            data=json.dumps(body, separators=(",", ":")).encode(),
            headers={
                "Authorization": f"Basic {self.otlp_auth}",
                "Content-Type": "application/json",
                "stream-name": "zpr_visa_service",
                "organization": self.openobserve_organization,
            },
            method="POST",
        )
        with urllib.request.urlopen(request, timeout=30) as response:
            response.read()

    def export_stats(self) -> None:
        response = self.admin_get("/admin/stats")
        stats = response.get("stats", {}) if isinstance(response, dict) else {}
        now_ns = time.time_ns()
        uptime_value = stats.get("uptime")
        try:
            uptime = float(uptime_value)
        except (TypeError, ValueError):
            uptime = None
        if uptime is not None and (self.last_uptime is None or uptime < self.last_uptime):
            self.metric_start_ns = now_ns - int(uptime * 1_000_000_000)
        if uptime is not None:
            self.last_uptime = uptime

        metrics = []
        for stat_name, raw_value in stats.items():
            if str(stat_name).lower() == "uptime":
                continue
            try:
                numeric = float(raw_value)
            except (TypeError, ValueError):
                continue
            if not math.isfinite(numeric):
                continue
            datapoint: dict[str, object] = {
                "startTimeUnixNano": str(self.metric_start_ns),
                "timeUnixNano": str(now_ns),
                "asInt": str(int(numeric)) if numeric.is_integer() else str(numeric),
                "attributes": [otlp_string_attribute("zpr.stat.name", stat_name)],
            }
            metrics.append({
                "name": metric_name(str(stat_name)),
                "description": f"Visa Service statistic {stat_name}",
                "unit": "1",
                "sum": {"aggregationTemporality": 2, "isMonotonic": True, "dataPoints": [datapoint]},
            })
        if uptime is not None:
            metrics.append({
                "name": "zpr_vs_uptime_seconds",
                "description": "Visa Service process uptime",
                "unit": "s",
                "gauge": {"dataPoints": [{"timeUnixNano": str(now_ns), "asDouble": uptime}]},
            })
        if metrics:
            self.send_otlp("metrics", {
                "resourceMetrics": [{
                    "resource": self.resource,
                    "scopeMetrics": [{"scope": {"name": "zpr-observability-collector", "version": "1"}, "metrics": metrics}],
                }]
            })
            print(f"collector metrics exported: {len(metrics)} series", flush=True)

    def pull_denials(self) -> None:
        query = urllib.parse.urlencode({"since": max(0, self.last_deny_ms - 1000), "limit": 500})
        records = self.admin_get(f"/admin/visas/denies?{query}")
        if not isinstance(records, list):
            return
        for deny in records:
            if not isinstance(deny, dict):
                continue
            try:
                timestamp_ms = int(deny.get("last_deny_ms", 0))
            except (TypeError, ValueError):
                continue
            key = (
                timestamp_ms, deny.get("source_addr"), deny.get("dest_addr"),
                deny.get("protocol"), deny.get("dest_port"), deny.get("deny_code"), deny.get("count"),
            )
            if key in self.seen_denies or key in self.pending_denies:
                continue
            attributes = [
                otlp_string_attribute("event.name", "visa.denied"),
                otlp_string_attribute("zpr.deny_code", deny.get("deny_code", "unknown")),
                otlp_string_attribute("zpr.source_addr", deny.get("source_addr", "")),
                otlp_string_attribute("zpr.destination_addr", deny.get("dest_addr", "")),
                otlp_string_attribute("network.protocol.number", deny.get("protocol", "")),
                otlp_string_attribute("network.destination.port", deny.get("dest_port", "")),
                otlp_string_attribute("zpr.denial.count", deny.get("count", "")),
            ]
            record = {
                "timeUnixNano": str(timestamp_ms * 1_000_000),
                "severityNumber": 13,
                "severityText": "WARN",
                "body": {"stringValue": f"Visa denied: {deny.get('deny_code', 'unknown')}"},
                "attributes": attributes,
            }
            self.pending_logs.append((record, key, timestamp_ms))
            self.pending_denies.add(key)

    def find_vs_log(self) -> Path | None:
        configured = os.environ.get("ZPR_VS_LOG_FILE")
        if configured:
            path = Path(configured)
            return path if path.is_file() else None
        try:
            entries = list(Path("/proc").iterdir())
        except OSError:
            return None
        for entry in entries:
            if not entry.name.isdigit():
                continue
            try:
                if (entry / "comm").read_text(encoding="utf-8").strip() != "vs":
                    continue
                args = (entry / "cmdline").read_bytes().replace(b"\0", b" ")
                if b"zpr-vs-target/debug/vs" not in args:
                    continue
                path = Path(os.readlink(entry / "cwd")) / "vs.log"
                return path if path.is_file() else None
            except (OSError, ProcessLookupError):
                continue
        return None

    def tail_vs_logs(self) -> None:
        path = self.find_vs_log()
        if path is None:
            return
        try:
            info = path.stat()
            identity = (info.st_dev, info.st_ino)
            if path != self.log_path or identity != self.log_identity or info.st_size < self.log_offset:
                self.log_path = path
                self.log_identity = identity
                self.log_offset = info.st_size
                return
            with path.open("rb") as source:
                source.seek(self.log_offset)
                lines = source.readlines(256 * 1024)
                self.log_offset = source.tell()
        except OSError:
            return
        now_ns = time.time_ns()
        for raw_line in lines:
            message = redact(raw_line.decode("utf-8", errors="replace").rstrip())
            if not message:
                continue
            match = re.search(r"\b(TRACE|DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL)\b", message, re.IGNORECASE)
            severity = (match.group(1).upper() if match else "INFO").replace("WARNING", "WARN")
            severity_number = {"TRACE": 1, "DEBUG": 5, "INFO": 9, "WARN": 13, "ERROR": 17, "FATAL": 21}[severity]
            record = {
                "timeUnixNano": str(now_ns),
                "severityNumber": severity_number,
                "severityText": severity,
                "body": {"stringValue": message},
                "attributes": [otlp_string_attribute("event.name", "visa_service.log")],
            }
            self.pending_logs.append((record, None, 0))
        if len(self.pending_logs) > MAX_PENDING_LOGS:
            del self.pending_logs[:-MAX_PENDING_LOGS]

    def flush_logs(self) -> None:
        while self.pending_logs:
            batch = self.pending_logs[:MAX_LOG_BATCH]
            body = {
                "resourceLogs": [{
                    "resource": self.resource,
                    "scopeLogs": [{"scope": {"name": "zpr-observability-collector", "version": "1"}, "logRecords": [item[0] for item in batch]}],
                }]
            }
            self.send_otlp("logs", body)
            print(f"collector logs exported: {len(batch)} records", flush=True)
            for _, key, timestamp_ms in batch:
                if key is not None:
                    self.pending_denies.discard(key)
                    self.seen_denies.add(key)
                    self.last_deny_ms = max(self.last_deny_ms, timestamp_ms)
            del self.pending_logs[:len(batch)]
            if len(self.seen_denies) > 10000:
                self.seen_denies.clear()

    def run(self, once: bool = False) -> None:
        while True:
            for label, operation in (
                ("stats", self.export_stats),
                ("denials", self.pull_denials),
                ("logs", self.tail_vs_logs),
                ("log export", self.flush_logs),
            ):
                try:
                    operation()
                except Exception as error:  # telemetry failures must not affect authorization
                    print(f"collector {label} warning: {type(error).__name__}: {error}", file=sys.stderr, flush=True)
            if once:
                return
            time.sleep(POLL_SECONDS)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--once", action="store_true", help="collect one polling interval and exit")
    args = parser.parse_args()
    runtime = Path(os.environ.get("ZPR_RUNTIME_DIR", str(DEFAULT_RUNTIME)))
    Collector(runtime).run(once=args.once)


if __name__ == "__main__":
    main()