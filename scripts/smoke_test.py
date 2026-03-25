#!/usr/bin/env python3
"""Minimal end-to-end smoke test for FlowGate."""

from __future__ import annotations

import argparse
import json
import os
import shutil
import socket
import ssl
import subprocess
import sys
import tempfile
import threading
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


def read_exact(sock: socket.socket, size: int, timeout: float = 5.0) -> bytes:
    sock.settimeout(timeout)
    chunks = []
    remaining = size
    while remaining > 0:
        chunk = sock.recv(remaining)
        if not chunk:
            raise SmokeFailure(f"expected {size} bytes, received {size - remaining}")
        chunks.append(chunk)
        remaining -= len(chunk)
    return b"".join(chunks)


def read_until(sock: socket.socket, marker: bytes, timeout: float = 5.0) -> bytes:
    sock.settimeout(timeout)
    data = bytearray()
    while marker not in data:
        chunk = sock.recv(1)
        if not chunk:
            raise SmokeFailure(f"connection closed before marker {marker!r}")
        data.extend(chunk)
    return bytes(data)


class ManagedTCPServer:
    def __init__(self, name: str):
        self.name = name
        self.sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.sock.bind(("127.0.0.1", 0))
        self.sock.listen()
        self.sock.settimeout(0.5)
        self.port = int(self.sock.getsockname()[1])
        self.accept_count = 0
        self.errors: list[str] = []
        self._stop = threading.Event()
        self._connections: set[socket.socket] = set()
        self._connections_lock = threading.Lock()
        self._thread = threading.Thread(target=self._accept_loop, daemon=True)
        self._thread.start()

    def _accept_loop(self) -> None:
        while not self._stop.is_set():
            try:
                conn, _ = self.sock.accept()
            except socket.timeout:
                continue
            except OSError:
                if self._stop.is_set():
                    return
                self.errors.append("accept failed")
                return

            self.accept_count += 1
            with self._connections_lock:
                self._connections.add(conn)
            thread = threading.Thread(target=self._handle_conn_wrapper, args=(conn,), daemon=True)
            thread.start()

    def _handle_conn_wrapper(self, conn: socket.socket) -> None:
        try:
            self.handle_conn(conn)
        except Exception as exc:  # noqa: BLE001
            self.errors.append(f"{self.name}: {exc}")
        finally:
            with self._connections_lock:
                self._connections.discard(conn)
            try:
                conn.close()
            except OSError:
                pass

    def handle_conn(self, conn: socket.socket) -> None:
        raise NotImplementedError

    def stop(self) -> None:
        self._stop.set()
        try:
            self.sock.close()
        except OSError:
            pass
        with self._connections_lock:
            for conn in list(self._connections):
                try:
                    conn.close()
                except OSError:
                    pass
            self._connections.clear()
        self._thread.join(timeout=2)

    def assert_healthy(self) -> None:
        if self.errors:
            raise SmokeFailure(f"{self.name} errors: {'; '.join(self.errors)}")


class EchoTCPServer(ManagedTCPServer):
    def __init__(self, name: str):
        super().__init__(name)
        self.payloads: list[bytes] = []

    def handle_conn(self, conn: socket.socket) -> None:
        conn.settimeout(5)
        while not self._stop.is_set():
            data = conn.recv(65536)
            if not data:
                return
            self.payloads.append(data)
            conn.sendall(data)


class ProxyCaptureServer(ManagedTCPServer):
    def __init__(self, name: str, version: int, expected_payload: bytes):
        super().__init__(name)
        self.version = version
        self.expected_payload = expected_payload
        self.headers: list[bytes] = []
        self.payloads: list[bytes] = []

    def handle_conn(self, conn: socket.socket) -> None:
        conn.settimeout(5)
        if self.version == 1:
            header = read_until(conn, b"\r\n")
        else:
            header = read_exact(conn, 28)
        payload = read_exact(conn, len(self.expected_payload))
        self.headers.append(header)
        self.payloads.append(payload)
        conn.sendall(b"proxy-ok")


def tcp_roundtrip(port: int, payload: bytes, *, timeout: float = 5.0) -> bytes:
    with socket.create_connection(("127.0.0.1", port), timeout=timeout) as sock:
        sock.settimeout(timeout)
        sock.sendall(payload)
        return read_exact(sock, len(payload), timeout=timeout)


def tcp_request_expect_close(port: int, payload: bytes, *, timeout: float = 2.0) -> bytes:
    with socket.create_connection(("127.0.0.1", port), timeout=timeout) as sock:
        sock.settimeout(timeout)
        sock.sendall(payload)
        try:
            return sock.recv(4096)
        except (ConnectionResetError, socket.timeout):
            return b""


def tls_client_roundtrip(port: int, payload: bytes, *, timeout: float = 5.0) -> bytes:
    context = ssl.create_default_context()
    context.check_hostname = False
    context.verify_mode = ssl.CERT_NONE

    raw_sock = socket.create_connection(("127.0.0.1", port), timeout=timeout)
    with context.wrap_socket(raw_sock, server_hostname="127.0.0.1") as sock:
        sock.settimeout(timeout)
        sock.sendall(payload)
        return read_exact(sock, len(payload), timeout=timeout)


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


def build_helper_binary(go_bin: str, output_path: Path) -> None:
    command = [go_bin, "build", "-o", str(output_path), "./scripts/tunnel_helper.go"]
    subprocess.run(command, cwd=REPO_ROOT, check=True)


def ensure_local_build_prereqs(binary_supplied: bool) -> None:
    if binary_supplied or os.name != "nt":
        return
    if shutil.which("gcc") or shutil.which("clang"):
        return
    raise SmokeFailure(
        "Local FlowGate builds on Windows require a C compiler for SQLite (for example gcc/clang). "
        "Pass --binary with a prebuilt executable or run the smoke test on Linux."
    )


def create_rule(base_url: str, admin_token: str, payload: dict, description: str) -> dict:
    status, body, raw = http_request(
        base_url + "/api/rules",
        method="POST",
        token=admin_token,
        payload=payload,
    )
    ensure_status(status, 200, raw, description)
    return body["rule"]


def wait_for_rule_running(
    base_url: str,
    admin_token: str,
    rule_id: int,
    *,
    processes: list[ManagedProcess],
    timeout: float = 25.0,
) -> dict:
    return wait_for(
        f"rule {rule_id} running",
        lambda: next(
            (
                rule
                for rule in http_request(base_url + "/api/rules", token=admin_token)[1]["rules"]
                if rule["id"] == rule_id and rule["runtime_status"] == "running"
            ),
            None,
        ),
        timeout=timeout,
        processes=processes,
    )


def run_helper(
    helper_binary: Path,
    *args: str,
    timeout: float = 10.0,
) -> bytes:
    result = subprocess.run(
        [str(helper_binary), *args],
        cwd=REPO_ROOT,
        capture_output=True,
        timeout=timeout,
        check=False,
    )
    if result.returncode != 0:
        stderr = result.stderr.decode("utf-8", errors="replace")
        stdout = result.stdout.decode("utf-8", errors="replace")
        raise SmokeFailure(
            f"helper {' '.join(args)} failed with {result.returncode}\nstdout:\n{stdout}\nstderr:\n{stderr}"
        )
    return result.stdout


def run_phase1_scenarios(
    *,
    base_url: str,
    admin_token: str,
    node_id: int,
    processes: list[ManagedProcess],
    helper_binary: Path,
    logs_dir: Path,
) -> None:
    # Unsupported config rejection: WS + inbound TLS must fail at API layer.
    status, _, raw = http_request(
        base_url + "/api/rules",
        method="POST",
        token=admin_token,
        payload={
            "node_id": node_id,
            "name": "invalid-ws-tls",
            "protocol": "tcp",
            "listen_port": find_free_port(),
            "target_addr": "127.0.0.1",
            "target_port": find_free_port(),
            "speed_limit": 0,
            "ws_enabled": True,
            "tls_mode": "client",
        },
    )
    ensure_status(status, 400, raw, "invalid ws+tls rule creation")

    extra_servers: list[ManagedTCPServer] = []
    try:
        # Inbound TLS rule: client connects with TLS to the rule port.
        inbound_target = EchoTCPServer("phase1-inbound-tls-target")
        extra_servers.append(inbound_target)
        inbound_listen = find_free_port()
        inbound_rule = create_rule(
            base_url,
            admin_token,
            {
                "node_id": node_id,
                "name": "phase1-inbound-tls",
                "protocol": "tcp",
                "listen_port": inbound_listen,
                "target_addr": "127.0.0.1",
                "target_port": inbound_target.port,
                "speed_limit": 0,
                "tls_mode": "client",
            },
            "inbound tls rule creation",
        )
        wait_for_rule_running(base_url, admin_token, inbound_rule["id"], processes=processes)
        inbound_payload = b"inbound tls ok"
        if tls_client_roundtrip(inbound_listen, inbound_payload) != inbound_payload:
            raise SmokeFailure("inbound TLS roundtrip returned unexpected payload")

        # Outbound TLS rule: target is a TLS echo server, client connects in plain TCP.
        outbound_target_port = find_free_port()
        outbound_target = ManagedProcess(
            "phase1-outbound-tls-target",
            [str(helper_binary), "tls-echo-server", "--addr", f"127.0.0.1:{outbound_target_port}"],
            logs_dir / "phase1-outbound-tls-target.log",
            cwd=REPO_ROOT,
        )
        processes.append(outbound_target)
        wait_for(
            "outbound TLS target ready",
            lambda: socket.create_connection(("127.0.0.1", outbound_target_port), timeout=1).close() or True,
            timeout=10,
            interval=0.2,
            processes=processes,
        )
        outbound_listen = find_free_port()
        outbound_rule = create_rule(
            base_url,
            admin_token,
            {
                "node_id": node_id,
                "name": "phase1-outbound-tls",
                "protocol": "tcp",
                "listen_port": outbound_listen,
                "target_addr": "127.0.0.1",
                "target_port": outbound_target_port,
                "speed_limit": 0,
                "tls_mode": "server",
            },
            "outbound tls rule creation",
        )
        wait_for_rule_running(base_url, admin_token, outbound_rule["id"], processes=processes)
        outbound_payload = b"outbound tls ok"
        if tcp_roundtrip(outbound_listen, outbound_payload) != outbound_payload:
            raise SmokeFailure("outbound TLS roundtrip returned unexpected payload")

        # WebSocket-only tunnel rule.
        ws_target = EchoTCPServer("phase1-ws-target")
        extra_servers.append(ws_target)
        ws_listen = find_free_port()
        ws_rule = create_rule(
            base_url,
            admin_token,
            {
                "node_id": node_id,
                "name": "phase1-ws-only",
                "protocol": "tcp",
                "listen_port": ws_listen,
                "target_addr": "127.0.0.1",
                "target_port": ws_target.port,
                "speed_limit": 0,
                "ws_enabled": True,
                "ws_path": "/ws",
            },
            "ws tunnel rule creation",
        )
        wait_for_rule_running(base_url, admin_token, ws_rule["id"], processes=processes)
        ws_payload = b"phase1 ws ok"
        ws_reply = run_helper(
            helper_binary,
            "ws-roundtrip",
            "--url",
            f"ws://127.0.0.1:{ws_listen}/ws",
            "--message",
            ws_payload.decode("utf-8"),
        )
        if ws_reply != ws_payload:
            raise SmokeFailure(f"WebSocket tunnel returned unexpected payload: {ws_reply!r}")

        # HTTP protocol blocking should close the client and avoid dialing the target.
        blocked_target = EchoTCPServer("phase1-block-http-target")
        extra_servers.append(blocked_target)
        blocked_listen = find_free_port()
        blocked_rule = create_rule(
            base_url,
            admin_token,
            {
                "node_id": node_id,
                "name": "phase1-block-http",
                "protocol": "tcp",
                "listen_port": blocked_listen,
                "target_addr": "127.0.0.1",
                "target_port": blocked_target.port,
                "speed_limit": 0,
                "blocked_protos": "http",
            },
            "blocked http rule creation",
        )
        wait_for_rule_running(base_url, admin_token, blocked_rule["id"], processes=processes)
        blocked_reply = tcp_request_expect_close(
            blocked_listen,
            b"GET / HTTP/1.1\r\nHost: smoke\r\n\r\n",
        )
        if blocked_reply:
            raise SmokeFailure(f"blocked HTTP request unexpectedly received data: {blocked_reply!r}")
        time.sleep(0.5)
        if blocked_target.accept_count != 0:
            raise SmokeFailure(
                f"blocked HTTP rule still dialed target {blocked_target.accept_count} time(s)"
            )

        # PROXY protocol v1 and v2 header forwarding.
        for proxy_version in (1, 2):
            proxy_payload = f"proxy-v{proxy_version}".encode("utf-8")
            proxy_target = ProxyCaptureServer(
                f"phase1-proxy-v{proxy_version}-target",
                proxy_version,
                proxy_payload,
            )
            extra_servers.append(proxy_target)
            proxy_listen = find_free_port()
            proxy_rule = create_rule(
                base_url,
                admin_token,
                {
                    "node_id": node_id,
                    "name": f"phase1-proxy-v{proxy_version}",
                    "protocol": "tcp",
                    "listen_port": proxy_listen,
                    "target_addr": "127.0.0.1",
                    "target_port": proxy_target.port,
                    "speed_limit": 0,
                    "proxy_protocol": proxy_version,
                },
                f"proxy protocol v{proxy_version} rule creation",
            )
            wait_for_rule_running(base_url, admin_token, proxy_rule["id"], processes=processes)
            with socket.create_connection(("127.0.0.1", proxy_listen), timeout=5) as sock:
                sock.settimeout(5)
                sock.sendall(proxy_payload)
                reply = read_exact(sock, len(b"proxy-ok"))
            if reply != b"proxy-ok":
                raise SmokeFailure(f"unexpected PROXY v{proxy_version} reply: {reply!r}")
            wait_for(
                f"proxy v{proxy_version} capture",
                lambda: len(proxy_target.headers) == 1 and len(proxy_target.payloads) == 1,
                timeout=5,
                interval=0.2,
            )
            if proxy_target.payloads[0] != proxy_payload:
                raise SmokeFailure(
                    f"unexpected PROXY v{proxy_version} payload: {proxy_target.payloads[0]!r}"
                )
            header = proxy_target.headers[0]
            if proxy_version == 1:
                if not header.startswith(b"PROXY TCP4 "):
                    raise SmokeFailure(f"unexpected PROXY v1 header: {header!r}")
            elif header[:12] != b"\r\n\r\n\0\r\nQUIT\n":
                raise SmokeFailure(f"unexpected PROXY v2 signature: {header[:12]!r}")

        # Connection pool prefill should establish target connections eagerly and still relay traffic.
        pool_target = EchoTCPServer("phase1-pool-target")
        extra_servers.append(pool_target)
        pool_listen = find_free_port()
        pool_rule = create_rule(
            base_url,
            admin_token,
            {
                "node_id": node_id,
                "name": "phase1-pool",
                "protocol": "tcp",
                "listen_port": pool_listen,
                "target_addr": "127.0.0.1",
                "target_port": pool_target.port,
                "speed_limit": 0,
                "pool_size": 2,
            },
            "connection pool rule creation",
        )
        wait_for_rule_running(base_url, admin_token, pool_rule["id"], processes=processes)
        wait_for(
            "connection pool prefill",
            lambda: pool_target.accept_count >= 2,
            timeout=10,
            interval=0.2,
        )
        pool_payload = b"pool warm ok"
        if tcp_roundtrip(pool_listen, pool_payload) != pool_payload:
            raise SmokeFailure("connection pool roundtrip returned unexpected payload")

        for server in extra_servers:
            server.assert_healthy()
    finally:
        for server in reversed(extra_servers):
            server.stop()


def run_smoke(binary_path: Path, *, helper_binary: Path | None = None) -> None:
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

            if helper_binary is not None:
                run_phase1_scenarios(
                    base_url=base_url,
                    admin_token=admin_token,
                    node_id=node_id,
                    processes=processes,
                    helper_binary=helper_binary,
                    logs_dir=logs_dir,
                )

            print("Smoke test passed.")
            if helper_binary is not None:
                print("Phase 1 tunnel scenarios passed.")
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
    parser.add_argument(
        "--phase1",
        action="store_true",
        help="Run additional Phase 1 tunnel scenarios after the base smoke test.",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    temp_binary_dir: tempfile.TemporaryDirectory[str] | None = None
    temp_helper_dir: tempfile.TemporaryDirectory[str] | None = None

    try:
        ensure_local_build_prereqs(binary_supplied=bool(args.binary))
        if args.binary:
            binary_path = Path(args.binary).resolve()
        else:
            temp_binary_dir = tempfile.TemporaryDirectory(prefix="flowgate-binary-")
            binary_path = Path(temp_binary_dir.name) / (
                "flowgate.exe" if os.name == "nt" else "flowgate"
            )
            build_binary(args.go_bin, binary_path)

        helper_binary: Path | None = None
        if args.phase1:
            temp_helper_dir = tempfile.TemporaryDirectory(prefix="flowgate-helper-")
            helper_binary = Path(temp_helper_dir.name) / (
                "tunnel-helper.exe" if os.name == "nt" else "tunnel-helper"
            )
            build_helper_binary(args.go_bin, helper_binary)

        run_smoke(binary_path, helper_binary=helper_binary)
        return 0
    except (SmokeFailure, subprocess.CalledProcessError) as exc:
        print(str(exc), file=sys.stderr)
        return 1
    finally:
        if temp_binary_dir is not None:
            temp_binary_dir.cleanup()
        if temp_helper_dir is not None:
            temp_helper_dir.cleanup()


if __name__ == "__main__":
    sys.exit(main())
