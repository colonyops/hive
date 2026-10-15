import json
import os
from pathlib import Path
import ssl
import subprocess
import time
import urllib.request


def request(url, body=None, headers=None, context=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data, headers or {"Content-Type": "application/json"})
    with urllib.request.urlopen(req, context=context, timeout=10) as response:
        data = response.read()
        return json.loads(data) if data else None


def mcp(name, arguments):
    reply = request("http://127.0.0.1:19001/mcp", {
        "jsonrpc": "2.0", "id": 1, "method": "tools/call",
        "params": {"name": name, "arguments": arguments},
    }, {"Content-Type": "application/json", "Accept": "application/json, text/event-stream"})
    assert not reply.get("error") and not reply["result"].get("isError"), reply
    return reply["result"]["structuredContent"]


def wait(check):
    last = None
    for _ in range(120):
        try:
            if check():
                return
        except (OSError, ValueError) as error:
            last = error
        time.sleep(0.25)
    raise AssertionError(f"Timed out: {last}")


home = Path.home()
prefix = home / ".local/share/hive-server"
config = home / ".config/hive-server"
runtime = Path(os.environ["XDG_RUNTIME_DIR"])
runtime.mkdir(mode=0o700)
assert not os.getenv("DISPLAY") and not os.getenv("WAYLAND_DISPLAY")
subprocess.run(["/bundle/install.sh", "--no-start"], check=True)
env_file = config / "environment"
env_file.write_text(env_file.read_text() + "\n# preserved-install-test\n")
subprocess.run(["/bundle/install.sh", "--no-start"], check=True)
assert "preserved-install-test" in env_file.read_text()
assert env_file.stat().st_mode & 0o777 == 0o600
subprocess.run(["systemd-analyze", "--user", "verify", str(home / ".config/systemd/user/hive-server.service"), str(home / ".config/systemd/user/hive-server-tmux.service")], check=True)
workspace = config / "desktop/workspaces/shell"
workspace.mkdir(parents=True)
(workspace / "agent-workspace.yaml").write_text(json.dumps({
    "version": 5, "name": "Ubuntu shell", "command": "bash --noprofile --norc -i",
}))
scheduled = config / "desktop/workspaces/scheduled"
scheduled.mkdir(parents=True)
(home / "scheduled.sh").write_text('printf "%s\\n" "$1" > "$HOME/scheduled-result"\nsleep 15\n')
(scheduled / "agent-workspace.yaml").write_text(json.dumps({
    "version": 5, "name": "Ubuntu schedule",
    "command": "bash /home/hive/scheduled.sh {{ .Prompt | shq }}",
}))
flows = config / "desktop/flows"
flows.mkdir(parents=True, exist_ok=True)
(flows / "ubuntu.yaml").write_text(json.dumps({
    "version": 1, "name": "Ubuntu inbox", "nodes": [
        {"id": "webhook", "type": "sources.webhook", "path": "ubuntu"},
        {"id": "inbox", "type": "feed", "name": "Server"},
    ], "wires": [{"from": "webhook", "to": "inbox"}],
}))
launch = str(prefix / "launch.sh")
log = open("/tmp/hive-server-test.log", "w")
tmux = subprocess.Popen([launch, "tmux"], stdout=log, stderr=log)
server = None


def start():
    child = subprocess.Popen([launch], stdout=log, stderr=log)
    wait(lambda: child.poll() is None and request("http://127.0.0.1:19001/api/status"))
    return child


def api(credentials, path, body):
    return request(credentials["url"] + "/api/terminal/" + path, body,
        {"Content-Type": "application/json", "Authorization": "Bearer " + credentials["token"]},
        ssl.create_default_context(cadata=credentials["certificate"]))


try:
    subprocess.run([launch, "wait-tmux"], check=True)
    server = start()
    credential_file = prefix / "connection.json"
    wait(credential_file.exists)
    credentials = json.loads(credential_file.read_text())
    assert credential_file.stat().st_mode & 0o777 == 0o600
    assert credentials["url"].startswith("https://127.0.0.1:")
    chat = api(credentials, "agents/sessions/start", {"workspace": "shell", "name": "Survives restart"})
    assert chat["terminalId"]
    request("http://127.0.0.1:19001/hooks/ubuntu", {"id": "restart", "title": "Persisted Ubuntu item"})
    wait(lambda: len(mcp("list_inbox", {"profile": "ubuntu"})["items"]) == 1)
    mcp("put_schedule", {"workspace": "scheduled", "id": "probe", "cron": "@every 3s", "prompt": "ubuntu-scheduled-proof"})
    wait(lambda: mcp("schedule_runs", {"workspace": "scheduled", "id": "probe"})["runs"])
    mcp("put_schedule", {"workspace": "scheduled", "id": "probe", "disabled": True})
    run = mcp("schedule_runs", {"workspace": "scheduled", "id": "probe"})["runs"][0]
    assert run["status"] == "launched" and run["reason"] == "due"
    assert "ubuntu-scheduled-proof" in (home / "scheduled-result").read_text()
    identity = (prefix / "data/desktop/remote-tls/identity.pem").read_bytes()
    server.terminate()
    server.wait(timeout=30)
    assert tmux.poll() is None
    server = start()
    wait(lambda: json.loads(credential_file.read_text())["token"] != credentials["token"])
    credentials = json.loads(credential_file.read_text())
    assert (prefix / "data/desktop/remote-tls/identity.pem").read_bytes() == identity
    assert len(mcp("list_inbox", {"profile": "ubuntu"})["items"]) == 1
    assert mcp("schedule_runs", {"workspace": "scheduled", "id": "probe"})["runs"][0]["id"] == run["id"]
    resumed = api(credentials, "agents/sessions/resume", {"id": chat["id"]})
    assert resumed["terminalId"] == chat["terminalId"]
    print("PASS: Ubuntu without GUI/keychain, repeat install, private credentials, valid systemd units, HTTPS, scheduled execution, inbox/run persistence, and live tmux chat across backend restart")
finally:
    if server:
        server.terminate()
        server.wait(timeout=30)
    tmux.terminate()
    tmux.wait(timeout=10)
    log.close()
    # The isolated log holds no supplied credentials; print it only on failure.
    import sys
    if sys.exc_info()[0]:
        print(Path("/tmp/hive-server-test.log").read_text()[-12000:])
