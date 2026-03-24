#!/usr/bin/env python3
"""Minimal end-to-end smoke test for FlowGate."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent


class SmokeFailure(RuntimeError):
    pass


class ManagedProcess:
    def __init__(self, name: str, command: list[str], log_path: Path, cwd: Path | None = None):
        self.name = name
        self.command = command
        self.log_path = log_path
        self.log_file = log_path.open("wb")
        self.process = subprocess.Popen(
            command,
            cwd=str(cwd) if cwd else None,
            stdout=self.log_file,
            stderr=subprocess.STDOUT,
            start_new_session=(os.name != "nt"),
        )

    def stop(self) -> None:
        if self.process.poll() is None:
            self.process.terminate()
            try:
                self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait(timeout=5)
        self.log_file.close()

    def assert_running(self) -> None:
        code = self.process.poll()
        if code is not None:
            raise SmokeFailure(
                f"{self.name} exited unexpectedly with code {code}\n{read_log_tail(self.log_path)}"
            )


def read_log_tail(path: Path, max_chars: int = 4000) -> str:
    if not path.exists():
        return f"[missing log: {path}]"

    content = path.read_text(encoding="utf-8", errors="replace")
    if len(content) <= max_chars:
        return content
    return content[-max_chars:]


def find_free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def http_request(
    url: str,
    *,
    method: str = "GET",
    payload: dict | None = None,
    token: str | None = None,
    timeout: float = 10.0,
) -> tuple[int, dict | str, str]:
    headers = {}
    data = None

    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"

    if token:
        headers["Authorization"] = f"Bearer {token}"

    request = urllib.request.Request(url, data=data, method=method, headers=headers)

    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            raw = response.read().decode("utf-8")
            status = response.getcode()
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8")
        status = exc.code
    except urllib.error.URLError as exc:
        raise SmokeFailure(f"Request to {url} failed: {exc}") from exc

    try:
        body = json.loads(raw)
    except json.JSONDecodeError:
        body = raw

    return status, body, raw


def wait_for(
    description: str,
    callback,
    *,
    timeout: float = 20.0,
    interval: float = 0.5,
    processes: list[ManagedProcess] | None = None,
):
    deadline = time.time() + timeout
    last_error = None

    while time.time() < deadline:
        if processes:
            for process in processes:
                process.assert_running()

        try:
            result = callback()
            if result:
                return result
        except Exception as exc:  # noqa: BLE001
            last_error = exc

        time.sleep(interval)

    if last_error:
        raise SmokeFailure(f"Timed out waiting for {description}: {last_error}") from last_error
    raise SmokeFailure(f"Timed out waiting for {description}")


def ensure_status(
    actual_status: int,
    expected_status: int,
    raw_body: str,
    description: str,
) -> None:
    if actual_status != expected_status:
        raise SmokeFailure(
            f"{description} returned {actual_status}, expected {expected_status}\n{raw_body}"
        )


def build_binary(go_bin: str, output_path: Path) -> None:
    command = [go_bin, "build", "-o", str(output_path), "./cmd/flowgate"]
    subprocess.run(command, cwd=REPO_ROOT, check=True)


def run_smoke(binary_path: Path) -> None:
    with tempfile.TemporaryDirectory(prefix="flowgate-smoke-") as temp_dir:
        temp_root = Path(temp_dir)
        logs_dir = temp_root / "logs"
        logs_dir.mkdir(parents=True, exist_ok=True)

        db_path = temp_root / "flowgate.db"
        http_root = temp_root / "target"
        http_root.mkdir(parents=True, exist_ok=True)
        (http_root / "index.html").write_text("flowgate smoke ok\n", encoding="utf-8")

        panel_port = find_free_port()
        target_port = find_free_port()
        listen_port = find_free_port()
        jwt_secret = "flowgate-smoke-secret"
        admin_password = "  smoke admin 123  "
        user_password = "  smoke user 123  "

        processes: list[ManagedProcess] = []
        failure: Exception | None = None

        try:
            http_server = ManagedProcess(
                "http-target",
                [
                    sys.executable,
                    "-m",
                    "http.server",
                    str(target_port),
                    "--bind",
                    "127.0.0.1",
                    "--directory",
                    str(http_root),
                ],
                logs_dir / "http-target.log",
            )
            processes.append(http_server)

            panel = ManagedProcess(
                "panel",
                [
                    str(binary_path),
                    "panel",
                    "--host",
                    "127.0.0.1",
                    "--port",
                    str(panel_port),
                    "--db",
                    str(db_path),
                    "--secret",
                    jwt_secret,
                ],
                logs_dir / "panel.log",
                cwd=REPO_ROOT,
            )
            processes.append(panel)

            base_url = f"http://127.0.0.1:{panel_port}"
            ws_url = f"ws://127.0.0.1:{panel_port}/ws/node"

            wait_for(
                "panel readiness",
                lambda: http_request(base_url + "/api/auth/setup")[1].get("needs_setup") is True,
                processes=processes,
            )

            status, body, raw = http_request(
                base_url + "/api/auth/register",
                method="POST",
                payload={"username": "admin", "password": admin_password},
            )
            ensure_status(status, 200, raw, "bootstrap registration")
            if not body.get("token"):
                raise SmokeFailure(f"bootstrap registration did not return a token:\n{raw}")

            status, _, raw = http_request(
                base_url + "/api/auth/register",
                method="POST",
                payload={"username": "admin2", "password": "another123"},
            )
            ensure_status(status, 403, raw, "second registration attempt")

            status, body, raw = http_request(
                base_url + "/api/auth/login",
                method="POST",
                payload={"username": "admin", "password": admin_password},
            )
            ensure_status(status, 200, raw, "admin login")
            admin_token = body["token"]
            if not admin_token:
                raise SmokeFailure("admin login did not return a token")

            status, body, raw = http_request(
                base_url + "/api/users",
                method="POST",
                payload={"username": "tester", "password": user_password},
                token=admin_token,
            )
            ensure_status(status, 200, raw, "user creation")
            if body["user"]["username"] != "tester":
                raise SmokeFailure(f"unexpected created user payload: {raw}")

            status, _, raw = http_request(
                base_url + "/api/auth/login",
                method="POST",
                payload={"username": "tester", "password": user_password},
            )
            ensure_status(status, 200, raw, "new user login")

            status, body, raw = http_request(
                base_url + "/api/nodes",
                method="POST",
                payload={"name": "smoke-node", "group_name": "ci"},
                token=admin_token,
            )
            ensure_status(status, 200, raw, "node creation")
            node_id = body["node"]["id"]
            api_key = body["node"]["api_key"]

            node = ManagedProcess(
                "node",
                [str(binary_path), "node", "--panel", ws_url, "--key", api_key],
                logs_dir / "node.log",
                cwd=REPO_ROOT,
            )
            processes.append(node)

            wait_for(
                "node online",
                lambda: any(
                    n["name"] == "smoke-node" and n["status"] == "online"
                    for n in http_request(base_url + "/api/nodes", token=admin_token)[1]["nodes"]
                ),
                processes=processes,
            )

            status, body, raw = http_request(
                base_url + "/api/rules",
                method="POST",
                token=admin_token,
                payload={
                    "node_id": node_id,
                    "name": "smoke-tcp",
                    "protocol": "tcp",
                    "listen_port": listen_port,
                    "target_addr": "127.0.0.1",
                    "target_port": target_port,
                    "speed_limit": 0,
                },
            )
            ensure_status(status, 200, raw, "rule creation")
            rule_id = body["rule"]["id"]

            wait_for(
                "rule running",
                lambda: next(
                    (
                        rule
                        for rule in http_request(base_url + "/api/rules", token=admin_token)[1]["rules"]
                        if rule["id"] == rule_id and rule["runtime_status"] == "running"
                    ),
                    None,
                ),
                timeout=25,
                processes=processes,
            )

            with urllib.request.urlopen(f"http://127.0.0.1:{listen_port}/", timeout=10) as response:
                body_text = response.read().decode("utf-8")
            if "flowgate smoke ok" not in body_text:
                raise SmokeFailure(f"unexpected forwarded response body:\n{body_text}")

            status, body, raw = http_request(base_url + "/api/events?limit=10", token=admin_token)
            ensure_status(status, 200, raw, "event feed query")
            if not any(event["category"] == "rule" for event in body["events"]):
                raise SmokeFailure(f"event feed did not include rule events:\n{raw}")

            print("Smoke test passed.")
            print(f"Panel URL: {base_url}")
            print(f"Rule port: {listen_port}")
        except Exception as exc:  # noqa: BLE001
            failure = exc
            raise
        finally:
            for process in reversed(processes):
                process.stop()
            if failure is not None:
                for process in processes:
                    print(
                        f"\n===== {process.name} log =====\n{read_log_tail(process.log_path)}",
                        file=sys.stderr,
                    )


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run a minimal FlowGate smoke test")
    parser.add_argument(
        "--binary",
        help="Path to a prebuilt flowgate binary. If omitted, the script builds one with go build.",
    )
    parser.add_argument(
        "--go-bin",
        default=shutil.which("go") or "go",
        help="Go executable to use when building a temporary binary.",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    temp_binary_dir: tempfile.TemporaryDirectory[str] | None = None

    try:
        if args.binary:
            binary_path = Path(args.binary).resolve()
        else:
            temp_binary_dir = tempfile.TemporaryDirectory(prefix="flowgate-binary-")
            binary_path = Path(temp_binary_dir.name) / (
                "flowgate.exe" if os.name == "nt" else "flowgate"
            )
            build_binary(args.go_bin, binary_path)

        run_smoke(binary_path)
        return 0
    except (SmokeFailure, subprocess.CalledProcessError) as exc:
        print(str(exc), file=sys.stderr)
        return 1
    finally:
        if temp_binary_dir is not None:
            temp_binary_dir.cleanup()


if __name__ == "__main__":
    sys.exit(main())
