"""Import a ChatGPT auth cache through stdin without exposing it in logs."""

import json
import os
from pathlib import Path
import sys
import tempfile


def main():
    try:
        data = json.load(sys.stdin)
        tokens = data.get("tokens", {})
        if data.get("auth_mode") not in (None, "chatgpt") or not isinstance(tokens, dict):
            raise ValueError()
        if not all(tokens.get(key) for key in ("access_token", "refresh_token", "id_token")):
            raise ValueError()
        if data.get("OPENAI_API_KEY"):
            raise ValueError()
    except (ValueError, AttributeError):
        sys.exit("Expected a Codex ChatGPT auth cache. Run codex login locally first.")

    directory = Path.home() / ".codex"
    directory.mkdir(mode=0o700, exist_ok=True)
    directory.chmod(0o700)
    fd, temporary = tempfile.mkstemp(prefix=".auth-", dir=directory)
    try:
        with os.fdopen(fd, "w") as file:
            json.dump(data, file)
        os.replace(temporary, directory / "auth.json")
    finally:
        Path(temporary).unlink(missing_ok=True)
    print("Imported ChatGPT credentials into the remote fixture (auth.json mode 0600).")


if __name__ == "__main__":
    main()
