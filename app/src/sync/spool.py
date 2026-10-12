"""Local spool for metrics that could not be written to Supabase.

Design:
- one JSON file per batch (kind + rows), named with a millisecond timestamp and
  a random suffix so ordering is naturally FIFO by name;
- a .sha256 sidecar with the checksum of the payload;
- a hard size cap (LOCAL_SPOOL_MAX_MB). When exceeded we DO NOT drop data
  silently: the caller is told and must alert; nothing is deleted before commit.
"""

from __future__ import annotations

import datetime as dt
import hashlib
import json
import uuid
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, Iterator, List, Tuple


@dataclass
class SpoolStatus:
    files: int
    bytes: int
    mb: float
    max_mb: int

    @property
    def over_limit(self) -> bool:
        return self.mb >= self.max_mb


def _json_default(value):
    if isinstance(value, dt.datetime):
        return value.isoformat()
    if isinstance(value, dt.date):
        return value.isoformat()
    raise TypeError(f"not JSON serializable: {type(value)!r}")


class Spool:
    def __init__(self, directory: str | Path, max_mb: int = 500):
        self.directory = Path(directory)
        self.max_mb = max(1, int(max_mb))

    def _ensure_dir(self) -> None:
        self.directory.mkdir(parents=True, exist_ok=True)
    def write_batch(self, kind: str, rows: Iterable[dict]) -> Path:
        self._ensure_dir()
        payload = {
            "kind": kind,
            "created_at_utc": dt.datetime.now(dt.timezone.utc).isoformat(),
            "rows": list(rows),
        }
        data = json.dumps(payload, default=_json_default).encode("utf-8")
        digest = hashlib.sha256(data).hexdigest()
        name = f"{kind}-{int(dt.datetime.now().timestamp() * 1000)}-{uuid.uuid4().hex[:8]}.json"
        path = self.directory / name
        path.write_bytes(data)
        path.with_suffix(".sha256").write_text(digest, encoding="ascii")
        return path

    def iter_pending(self) -> Iterator[Tuple[Path, str, List[dict]]]:
        if not self.directory.exists():
            return
        for path in sorted(self.directory.glob("*.json")):
            try:
                data = path.read_bytes()
                sidecar = path.with_suffix(".sha256")
                if sidecar.exists():
                    expected = sidecar.read_text(encoding="ascii").strip()
                    if hashlib.sha256(data).hexdigest() != expected:
                        continue  # corrupt: leave it for manual inspection
                payload = json.loads(data.decode("utf-8"))
                yield path, payload.get("kind", "unknown"), payload.get("rows", [])
            except Exception:  # noqa: BLE001 - a corrupt file must not stop the loop
                continue

    def remove(self, path: Path) -> None:
        try:
            path.unlink(missing_ok=True)
            path.with_suffix(".sha256").unlink(missing_ok=True)
        except OSError:
            pass

    def status(self) -> SpoolStatus:
        if not self.directory.exists():
            return SpoolStatus(files=0, bytes=0, mb=0.0, max_mb=self.max_mb)
        files = [p for p in self.directory.glob("*.json") if p.is_file()]
        bytes_total = sum(p.stat().st_size for p in files)
        return SpoolStatus(
            files=len(files),
            bytes=bytes_total,
            mb=round(bytes_total / (1024 * 1024), 3),
            max_mb=self.max_mb,
        )

    def is_over_limit(self) -> bool:
        return self.status().over_limit