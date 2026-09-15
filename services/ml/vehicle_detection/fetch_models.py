"""Download the pinned model files and verify them.

    python fetch_models.py            # into ANPR_MODEL_DIR (default ./models)

Run once on a machine with internet access, then copy the models directory to
the edge server; the service itself never downloads anything.
"""

from __future__ import annotations

import hashlib
import io
import sys
import urllib.request
import zipfile

from model_registry import ALL_MODELS, MODEL_DIR, sha256_of, verify


def download(url: str) -> bytes:
    req = urllib.request.Request(url, headers={"User-Agent": "npdms-anpr-fetch/1"})
    with urllib.request.urlopen(req, timeout=600) as r:
        return r.read()


def main() -> int:
    MODEL_DIR.mkdir(parents=True, exist_ok=True)
    archives: dict[str, bytes] = {}
    failed = False
    for m in ALL_MODELS:
        if verify(m) is None:
            print(f"ok       {m.filename}")
            continue
        print(f"fetching {m.filename} from {m.url}")
        if m.archive_member:
            if m.url not in archives:
                blob = download(m.url)
                digest = hashlib.sha256(blob).hexdigest()
                if digest != m.archive_sha256:
                    print(f"  archive digest {digest} does not match {m.archive_sha256}", file=sys.stderr)
                    failed = True
                    continue
                archives[m.url] = blob
            data = zipfile.ZipFile(io.BytesIO(archives[m.url])).read(m.archive_member)
        else:
            data = download(m.url)
        tmp = m.path.with_suffix(".part")
        tmp.write_bytes(data)
        if sha256_of(tmp) != m.sha256:
            tmp.unlink()
            print(f"  {m.filename} digest mismatch; not installed", file=sys.stderr)
            failed = True
            continue
        tmp.replace(m.path)
        print(f"  installed {m.filename}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
