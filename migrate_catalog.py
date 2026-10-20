#!/usr/bin/env python3
"""One-time, read-only import of the earlier SQLite catalog into the Go app."""
import argparse
from contextlib import closing
import json
from pathlib import Path
import sqlite3


def migrate(directory):
    source = directory / 'catalog.sqlite3'
    destination = directory / 'catalog.json'
    with closing(sqlite3.connect(source.resolve().as_uri() + '?mode=ro', uri=True)) as db:
        db.row_factory = sqlite3.Row
        root = db.execute("SELECT value FROM settings WHERE key='root'").fetchone()
        images = {}
        for row in db.execute('SELECT * FROM images'):
            image = dict(row)
            image['present'] = bool(image['present'])
            images[image['path']] = image
    result = {'version': 1, 'root': root[0] if root else '', 'images': images, 'albums': {}}
    # Exclusive creation prevents replacing a newer catalog or existing notes.
    with destination.open('x', encoding='utf-8') as output:
        json.dump(result, output, ensure_ascii=False, indent=2)
    print(f'Imported {len(images)} records to {destination}. The SQLite file is unchanged.')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--data', type=Path, default=Path.home() / '.soft-inventory')
    args = parser.parse_args()
    migrate(args.data.expanduser())
