# -*- coding: utf-8 -*-
"""Merge artist name variants into one canonical spelling.

Dry run by default; --apply performs the update in one transaction.

Canonical names (confirmed by the library owner):
    БИ-2 / Би-2 / "БИ-2\\t"   -> Би-2
    СпЛин / Сплин              -> Сплин
    DDT / " DDT" / ДДТ         -> ДДТ
    ПОРНОФИЛЬМЫ / Порнофильмы  -> Порнофильмы
    Jay-Z / Linkin Park        -> Jay-Z/Linkin Park
    Юрий Шевчук & Константин Казански -> ДДТ

Also: strip stray whitespace/tabs on every artist, and fix look-alike
spellings inside combined credits (e.g. "Thomas/БИ-2/Сплин").

After --apply call the player: POST /api/reload (rebuilds index and
entity_vectors from tracks.artist).
"""
import argparse
import sqlite3
import sys

DB = r"M:\AI_Musik\data\db\musik.db"

# exact (after strip) replacements
EXACT = {
    "БИ-2": "Би-2",
    "Би-2": "Би-2",
    "СпЛин": "Сплин",
    "Сплин": "Сплин",
    "DDT": "ДДТ",
    "ДДТ": "ДДТ",
    "ПОРНОФИЛЬМЫ": "Порнофильмы",
    "Порнофильмы": "Порнофильмы",
    "Jay-Z / Linkin Park": "Jay-Z/Linkin Park",
    "Jay-Z/Linkin Park": "Jay-Z/Linkin Park",
    "Юрий Шевчук & Константин Казански": "ДДТ",
}

# look-alike fixes inside combined credits
SUBSTRING = [
    ("БИ-2", "Би-2"),
    ("СпЛин", "Сплин"),
]


def normalize(artist: str) -> str:
    a = (artist or "").strip(" \t\r\n")
    if a in EXACT:
        return EXACT[a]
    changed = False
    for old, new in SUBSTRING:
        if old in a:
            a = a.replace(old, new)
            changed = True
    if changed:
        return a
    return artist.strip(" \t\r\n") if artist != artist.strip(" \t\r\n") else artist


def tables_with(db: sqlite3.Connection, column: str) -> list[str]:
    out = []
    for (name,) in db.execute(
        "SELECT name FROM sqlite_master WHERE type='table'"
    ).fetchall():
        cols = {r[1] for r in db.execute(f"PRAGMA table_info({name})")}
        if column in cols:
            out.append(name)
    return out


def main() -> int:
    ap = argparse.ArgumentParser(description="Merge artist name variants")
    ap.add_argument("--apply", action="store_true", help="perform the update")
    args = ap.parse_args()

    db = sqlite3.connect(DB)
    db.row_factory = sqlite3.Row

    plans: dict[str, list[tuple[str, int]]] = {}
    for table, col in (("tracks", "artist"), ("discover_tips", "artist"),
                       ("favorite_artists", "artist"), ("favorite_albums", "artist")):
        if table not in tables_with(db, col):
            continue
        try:
            rows = db.execute(
                f"SELECT {col} AS a, COUNT(*) AS n FROM {table} "
                f"WHERE {col} IS NOT NULL GROUP BY {col}"
            ).fetchall()
        except sqlite3.Error:
            continue
        for r in rows:
            new = normalize(r["a"])
            if new != r["a"]:
                plans.setdefault(table, []).append((r["a"], new, r["n"]))
    # legacy playlist column (unresolved_artist), if present
    for t in tables_with(db, "unresolved_artist"):
        rows = db.execute(
            f"SELECT unresolved_artist AS a, COUNT(*) AS n FROM {t} "
            "WHERE unresolved_artist IS NOT NULL GROUP BY unresolved_artist"
        ).fetchall()
        for r in rows:
            new = normalize(r["a"])
            if new != r["a"]:
                plans.setdefault(t, []).append((r["a"], new, r["n"]))

    total = sum(len(v) for v in plans.values())
    if not total:
        print("nothing to merge")
        return 0
    for table, items in plans.items():
        print(f"[{table}]")
        for old, new, n in sorted(items, key=lambda x: -x[2]):
            print(f"  {n:5d}  {old!r} -> {new!r}")

    if not args.apply:
        print("\ndry run — nothing was changed. Re-run with --apply.")
        return 0

    with db:
        for table, items in plans.items():
            col = "unresolved_artist" if table.startswith("playlist_tracks") else "artist"
            for old, new, _n in items:
                db.execute(f"UPDATE {table} SET {col} = ? WHERE {col} = ?", (new, old))
    print(f"\napplied: {total} mapping(s) across {len(plans)} table(s)")
    print("next: POST /api/reload on the player to rebuild index + entity_vectors")
    return 0


if __name__ == "__main__":
    sys.exit(main())
