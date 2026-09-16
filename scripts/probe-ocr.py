#!/usr/bin/env python3
"""Upload a scanned Bengali page and see whether an officer can find it.

This is the claim being tested end to end: OCR makes a scanned circular
findable in its own language, even though it does not read it well enough to
quote.
"""

import json
import mimetypes
import sys
import time
import urllib.error
import urllib.request
import uuid

BASE = sys.argv[1] if len(sys.argv) > 1 else "http://localhost:18082/api/v1"
PAGE = sys.argv[2] if len(sys.argv) > 2 else "ocr/page_ben.png"
PASSWORD = "Demo@123"

ok, fail = [], []


def check(name, condition, detail=""):
    (ok if condition else fail).append(name)
    print(("  ok   " if condition else "  FAIL ") + name + ("" if condition else " — " + str(detail)))


def call(method, path, token=None, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(BASE + path, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=120) as r:
            raw = r.read().decode()
            return r.status, (json.loads(raw) if raw else {})
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, {"raw": raw[:300]}


def upload(token, path, fields):
    """multipart/form-data, hand-rolled to avoid a dependency."""
    boundary = "----npdms" + uuid.uuid4().hex
    body = b""
    for key, value in fields.items():
        body += (f"--{boundary}\r\nContent-Disposition: form-data; name=\"{key}\"\r\n\r\n{value}\r\n").encode()
    with open(path, "rb") as f:
        content = f.read()
    ctype = mimetypes.guess_type(path)[0] or "application/octet-stream"
    body += (f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; "
             f"filename=\"{path.split('/')[-1]}\"\r\nContent-Type: {ctype}\r\n\r\n").encode()
    body += content + f"\r\n--{boundary}--\r\n".encode()

    req = urllib.request.Request(BASE + "/knowledge/documents", data=body, method="POST")
    req.add_header("Content-Type", f"multipart/form-data; boundary={boundary}")
    req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=180) as r:
            return r.status, json.loads(r.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        raw = e.read().decode()
        try:
            return e.code, json.loads(raw)
        except json.JSONDecodeError:
            return e.code, {"raw": raw[:400]}


def main():
    print(f"Probing {BASE} with {PAGE}")

    status, body = call("POST", "/auth/login", body={"username": "admin", "password": PASSWORD})
    if status != 200:
        print("cannot sign in:", status, body)
        sys.exit(1)
    token = body["accessToken"]

    print("\nWhat this server can read")
    status, caps = call("GET", "/knowledge/capabilities", token)
    check("capabilities read", status == 200, f"{status} {caps}")
    check("OCR is available here", caps.get("ocrAvailable") is True, str(caps.get("ocrAvailable")))
    langs = caps.get("ocrLanguages") or []
    indian = [l for l in ("ben", "hin", "mar", "tam", "tel", "kan", "mal", "guj", "pan", "ori", "asm", "urd") if l in langs]
    check("the languages of India are installed", len(indian) >= 10, f"only {indian}")
    print(f"       engine: {caps.get('ocrEngine')}")
    print(f"       Indian languages available: {' '.join(indian)}")

    print("\nFiling a scanned Bengali circular")
    reference = "PROBE-OCR-" + str(int(time.time()))
    status, doc = upload(token, PAGE, {"metadata": json.dumps({
        "title": "Probe: scanned Bengali circular",
        "docType": "CIRCULAR",
        "classification": "PUBLIC",
        "issuingAuthority": "Probe",
        "referenceNumber": reference,
        "issuedOn": "2026-09-16",
        "description": "Uploaded by the OCR probe. Safe to delete.",
    })})
    check("the scan was filed", status in (200, 201), f"{status} {doc}")
    if status not in (200, 201):
        sys.exit(1)

    doc_id = doc.get("id")
    check("the text was read by OCR", doc.get("extractionStatus") == "OCR",
          f"{doc.get('extractionStatus')}: {doc.get('extractionNote')}")
    print(f"       note: {doc.get('extractionNote')}")

    print("\nFinding it again, in Bengali")
    found_any = False
    for term in ("কলকাতা", "পুলিশ", "অভিযোগ", "থানা"):
        status, results = call("GET", f"/knowledge/documents?q={urllib.parse.quote(term)}", token)
        hits = results.get("data", []) if isinstance(results, dict) else []
        hit = any(h.get("id") == doc_id for h in hits)
        found_any = found_any or hit
        check(f"searching for {term} finds the scan", hit, f"{len(hits)} other result(s)")

    check("the scan is findable in its own language", found_any, "no Bengali term found it")

    print(f"\nTidy up:  psql -d npdms -c \"DELETE FROM knowledge_documents WHERE reference_number = '{reference}'\"")
    print(f"\n{len(ok)} passed, {len(fail)} failed")
    for f in fail:
        print("  FAILED " + f)
    sys.exit(1 if fail else 0)


if __name__ == "__main__":
    import urllib.parse
    main()
