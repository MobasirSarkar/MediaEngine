#!/usr/bin/env python3
"""
Smoke test for image-pipeline API.

Uploads a fake media file in 3 chunks, completes, and reports worker log activity.
"""
import hashlib
import json
import os
import time
import urllib.request
import subprocess

HOST = os.environ.get("HOST", "http://localhost:8080")
CHUNK_SIZE = 5 * 1024 * 1024
TOTAL_SIZE = CHUNK_SIZE * 3


def fetch(method, path, body=None, headers=None):
    headers = headers or {}
    headers.setdefault("Content-Type", "application/json")
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(f"{HOST}{path}", data=data, method=method, headers=headers)
    with urllib.request.urlopen(req) as r:
        return r.status, json.loads(r.read())


def raw_put(url, body, headers=None):
    headers = headers or {}
    headers.setdefault("Content-Type", "application/octet-stream")
    req = urllib.request.Request(url, data=body, method="PUT", headers=headers)
    with urllib.request.urlopen(req) as r:
        return r.status, r.read()


def step(name):
    print()
    print(f"\033[36m== {name} ==\033[0m")


def main():
    step("health")
    for p in ("/healthz", "/readyz"):
        s, b = fetch("GET", p)
        print(f"  {p} -> {s} {b}")

    print()
    step("create upload session")
    _, body = fetch("POST", "/uploads", {
        "owner_id": "smoke",
        "filename": "sample.bin",
        "content_type": "application/octet-stream",
        "total_size": TOTAL_SIZE,
        "chunk_size": CHUNK_SIZE,
    })
    print(json.dumps(body, indent=2))
    upload_id = body["data"]["upload_id"]
    urls      = body["data"]["chunk_urls"]
    print(f"  session {upload_id} ({len(urls)} chunks)")

    print()
    step("PUT + register chunks")
    for i, url in enumerate(urls):
        body_bytes = os.urandom(CHUNK_SIZE)
        digest     = hashlib.sha256(body_bytes).hexdigest()
        raw_put(url, body_bytes)
        s, b = fetch("POST", f"/uploads/{upload_id}/chunks/{i}", {
            "size": CHUNK_SIZE, "checksum": digest,
        })
        assert b.get("success"), b
        print(f"  ok chunk {i} {digest[:12]}...")

    print()
    step("complete upload")
    s, b = fetch("POST", f"/uploads/{upload_id}/complete", {})
    print(json.dumps(b, indent=2))

    print()
    step("wait for orchestrator")
    time.sleep(5)

    print()
    step("worker log tail")
    try:
        out = subprocess.check_output(["docker", "logs", "docker-worker-1", "--tail", "20"], stderr=subprocess.STDOUT)
        print(out.decode(errors="replace"))
    except Exception as e:
        print(f"  could not read worker logs: {e}")


if __name__ == "__main__":
    main()
