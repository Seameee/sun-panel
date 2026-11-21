#!/usr/bin/env python3
"""Upgrade a disposable database made by the official v1.3.0 binary in place."""
import argparse
import base64
import contextlib
import json
import secrets
import socket
import subprocess
import tempfile
import time
from pathlib import Path
from urllib.error import URLError
from urllib.request import Request, urlopen


PNG = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8A"
    "AwMCAO+a8xsAAAAASUVORK5CYII="
)


@contextlib.contextmanager
def running(binary, directory, url, log_name):
    with (directory / log_name).open("w+") as log:
        process = subprocess.Popen([binary], cwd=directory, stdout=log, stderr=log)
        try:
            for _ in range(150):
                if process.poll() is not None:
                    break
                try:
                    with urlopen(url, timeout=1):
                        break
                except URLError:
                    time.sleep(0.1)
            else:
                raise RuntimeError("server did not start")
            if process.poll() is not None:
                raise RuntimeError(f"server exited with status {process.returncode}")
            yield
        except Exception:
            log.seek(0)
            print(log.read()[-2500:])
            raise
        finally:
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()


def upgrade(old, new, directory, uploads, mysql):
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    url = f"http://127.0.0.1:{port}"
    (directory / "conf").mkdir()
    (directory / "web").mkdir()
    (directory / "web/index.html").write_text("Sun-Panel test")
    (directory / "conf/conf.ini").write_text(
        f"[base]\nhttp_address=127.0.0.1\nhttp_port={port}\n"
        f"database_drive={'mysql' if mysql else 'sqlite'}\n"
        f"cache_drive=memory\nqueue_drive=memory\nsource_path=./{uploads}\n"
        "source_temp_path=./runtime/temp\n[sqlite]\nfile_path=./database/database.db\n"
        + (f"[mysql]\nhost=127.0.0.1\nport={mysql[0]}\nusername=fixture\n"
           f"db_name={mysql[1]}\nwait_timeout=100\n" if mysql else "")
    )
    token = ""

    def post(route, params=None, content_type="application/json"):
        data = params if isinstance(params, bytes) else json.dumps(params or {}).encode()
        request = Request(url + "/api" + route, data=data,
                          headers={"Content-Type": content_type, "Token": token})
        with urlopen(request, timeout=5) as response:
            result = json.load(response)
        assert result["code"] == 0, (route, result["code"], result.get("msg"))
        return result.get("data")

    username = "admin@sun.cc"
    password = secrets.token_hex(12)
    group_route = "/panel/itemIconGroup"
    card_route = "/panel/itemIcon"
    with running(old, directory, url, "before.log"):
        account = post("/login", {"username": username, "password": "12345678"})
        token = account["token"]
        post("/user/updatePassword", {"oldPassword": "12345678", "newPassword": password})
        token = post("/login", {"username": username, "password": password})["token"]
        boundary = "compatibility-fixture"
        multipart = (f"--{boundary}\r\nContent-Disposition: form-data; name=\"imgfile\"; "
                     "filename=\"compat.png\"\r\nContent-Type: image/png\r\n\r\n").encode()
        image = post("/file/uploadImg", multipart + PNG + f"\r\n--{boundary}--\r\n".encode(),
                     f"multipart/form-data; boundary={boundary}")["imageUrl"]
        post("/user/updateInfo", {"name": "Legacy Admin", "headImage": image})
        group = post(group_route + "/edit", {"title": "原版分组", "sort": 2, "icon": "mdi:folder"})
        cards = [post(card_route + "/edit", {
            "itemIconGroupId": group["id"], "title": title, "description": "原版测试",
            "url": "https://example.com/wan", "lanUrl": "http://localhost/lan",
            "openMethod": 1, "icon": {"itemType": 2, "src": image, "text": "图标"},
        }) for title in ("原版链接一", "原版链接二")]
        post(card_route + "/saveSort", {"itemIconGroupId": group["id"], "sortItems": [
            {"id": cards[0]["id"], "sort": 2}, {"id": cards[1]["id"], "sort": 1},
        ]})
        expected = post(card_route + "/getListByGroupId", {"itemIconGroupId": group["id"]})["list"]
        assert [card["id"] for card in expected] == [cards[1]["id"], cards[0]["id"]]
        with urlopen(url + image, timeout=5) as response:
            assert response.read() == PNG

    for run in (1, 2):
        token = ""
        with running(new, directory, url, f"after-{run}.log"):
            session = post("/login", {"username": username, "password": password})
            assert session["id"] == account["id"] and session["role"] == account["role"]
            token = session["token"]
            profile = post("/user/getInfo")
            assert profile["name"] == "Legacy Admin" and profile["headImage"] == image
            security = post("/user/security/status")
            assert security["totpEnabled"] is False and security["passkeys"] == []
            groups = post(group_route + "/getList")["list"]
            matched = next(g for g in groups if g["id"] == group["id"])
            for field in ("title", "icon", "sort"):
                assert matched[field] == group[field], field
            actual = post(card_route + "/getListByGroupId", {"itemIconGroupId": group["id"]})["list"]
            assert len(actual) == len(expected)
            for got, want in zip(actual, expected):
                for field in ("id", "title", "url", "lanUrl", "sort", "description", "openMethod", "itemIconGroupId"):
                    assert got[field] == want[field], field
                for field in ("itemType", "src", "text"):
                    assert got["icon"][field] == want["icon"][field], f"icon.{field}"
            assert any(f["src"] == image for f in post("/file/getList")["list"])
            with urlopen(url + image, timeout=5) as response:
                assert response.read() == PNG
    backend = "MySQL" if mysql else "SQLite"
    print(f"v1.3.0 → current on {backend}, {uploads}: data preserved across two starts")


def main():
    args = argparse.ArgumentParser(description=__doc__)
    args.add_argument("old", type=lambda path: str(Path(path).resolve(strict=True)))
    args.add_argument("new", type=lambda path: str(Path(path).resolve(strict=True)))
    args.add_argument("--uploads", choices=("uploads", "legacy-files"), default="uploads")
    args.add_argument("--mysql", nargs=2, metavar=("PORT", "EMPTY_DATABASE"),
                      help="use a disposable empty DB on loopback with a passwordless fixture user")
    options = args.parse_args()
    with tempfile.TemporaryDirectory(prefix="sun-panel-upgrade-") as directory:
        upgrade(options.old, options.new, Path(directory), options.uploads, options.mysql)


if __name__ == "__main__":
    main()
