"""Batch cover-art fetch for the library."""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable

from rich.console import Console
from rich.progress import Progress

from musik.artwork.online import fetch_cover, throttle
from musik.config import get_settings
from musik.db.store import list_tracks_needing_artwork, save_artwork_path

logger = logging.getLogger(__name__)
console = Console()


@dataclass
class ArtworkResult:
    total: int = 0
    found: int = 0
    missing: int = 0
    failed: int = 0
    errors: list[str] = field(default_factory=list)


def save_cover(file_md5: str, data: bytes) -> str:
    out = get_settings().artwork_cache / f"{file_md5}.jpg"
    out.write_bytes(data)
    return str(out)


def fetch_library_artwork(
    *,
    limit: int | None = None,
    force: bool = False,
    delay_sec: float = 0.35,
    on_progress: Callable[[dict[str, Any]], None] | None = None,
) -> ArtworkResult:
    tracks = list_tracks_needing_artwork(limit=limit, force=force)
    if not force:
        # artwork_path may be stale (container path, deleted cache file)
        tracks = [
            t
            for t in tracks
            if not t.get("artwork_path") or not Path(str(t["artwork_path"])).is_file()
        ]
    result = ArtworkResult(total=len(tracks))
    if not tracks:
        console.print("[yellow]Нечего качать — обложки уже есть или библиотека пуста.[/yellow]")
        return result

    with Progress(console=console) as progress:
        task = progress.add_task("Artwork", total=len(tracks))
        for i, t in enumerate(tracks, start=1):
            tid = int(t["id"])
            try:
                hit = fetch_cover(
                    artist=t.get("artist") or "",
                    album=t.get("album") or "",
                )
                if hit is None:
                    result.missing += 1
                else:
                    path = save_cover(str(t["file_md5"]), hit)
                    save_artwork_path(tid, path)
                    result.found += 1
            except Exception as e:  # noqa: BLE001
                logger.exception("artwork failed track_id=%s", tid)
                result.failed += 1
                result.errors.append(f"{tid}: {e}")

            if on_progress is not None:
                on_progress(
                    {
                        "phase": "artwork",
                        "done": i,
                        "total": len(tracks),
                        "pct": round(100.0 * i / len(tracks), 1),
                        "message": (
                            f"artwork {i}/{len(tracks)} · "
                            f"found={result.found} missing={result.missing} fail={result.failed}"
                        ),
                    }
                )
            progress.update(
                task,
                advance=1,
                description=f"Artwork · found={result.found} miss={result.missing}",
            )
            throttle(delay_sec)

    return result
