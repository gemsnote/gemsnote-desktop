#!/usr/bin/env python3
"""Create a Finder drag-to-install disk image without GUI automation."""
import argparse
from pathlib import Path
import struct
import subprocess
import tempfile
import zlib


def write_background(path):
    """Draw a small, antialiased right arrow between the two Finder icons."""
    width, height = 660, 400
    paper, ink = (245, 248, 246), (55, 101, 80)
    rows = bytearray()
    for y in range(height):
        rows.append(0)  # PNG scanline filter: none
        for x in range(width):
            coverage = 0
            for dy in (0.125, 0.375, 0.625, 0.875):
                for dx in (0.125, 0.375, 0.625, 0.875):
                    px, py = x + dx, y + dy
                    shaft = 302 <= px <= 338 and 176 <= py <= 184
                    head = 330 <= px <= 354 and abs(py - 180) <= 354 - px
                    coverage += shaft or head
            rows.extend(round(a + (b - a) * coverage / 16) for a, b in zip(paper, ink))

    def chunk(kind, data):
        return (struct.pack('>I', len(data)) + kind + data
                + struct.pack('>I', zlib.crc32(kind + data) & 0xffffffff))

    path.write_bytes(b'\x89PNG\r\n\x1a\n'
                     + chunk(b'IHDR', struct.pack('>2I5B', width, height, 8, 2, 0, 0, 0))
                     + chunk(b'IDAT', zlib.compress(rows, 9)) + chunk(b'IEND', b''))


def settings(app, background):
    return {
        'format': 'UDZO',
        'files': [str(app)],
        'symlinks': {'Applications': '/Applications'},
        'background': str(background),
        'window_rect': ((200, 200), (660, 400)),
        'default_view': 'icon-view',
        'show_toolbar': False,
        'show_status_bar': False,
        'show_pathbar': False,
        'show_sidebar': False,
        'include_icon_view_settings': True,
        'include_list_view_settings': False,
        'arrange_by': None,
        'grid_spacing': 80,
        'icon_size': 96,
        'text_size': 14,
        'label_pos': 'bottom',
        'icon_locations': {app.name: (170, 180), 'Applications': (490, 180)},
        # Hiding an app extension adds FinderInfo to the signed bundle.
        # Keep its name intact so strict codesign verification still passes.
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('app', type=Path)
    parser.add_argument('volume_name')
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    app = args.app.resolve()
    if not (app / 'Contents/Info.plist').is_file():
        parser.error(f'Application bundle not found: {app}')
    try:
        import dmgbuild
    except ImportError:
        parser.error('Install scripts/dmg-requirements.txt with Python 3.10+; '
                     'set DMG_PYTHON to that environment for build-release.sh')
    # Preparation must already have sealed the final bundle. Never modify it.
    subprocess.run(['codesign', '--verify', '--deep', '--strict', str(app)], check=True)
    output = args.output.resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='gemsnote-dmg-') as temp:
        background = Path(temp) / 'background.png'
        write_background(background)
        dmgbuild.build_dmg(str(output), args.volume_name, settings=settings(app, background))


if __name__ == '__main__':
    main()
