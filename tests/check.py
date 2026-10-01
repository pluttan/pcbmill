"""End-to-end check: gerbers -> .tap -> probe -> map -> levelled .tap, plus G-code sanity.

Usage: python3.12 tests/check.py <gerber dir> [...]
"""
import glob
import math
import os
import re
import shutil
import subprocess
import sys
import tempfile
import tomllib

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, ROOT)
from pcbmill import board as bd, level  # noqa: E402

CFG = tomllib.load(open(os.path.join(ROOT, 'machine.toml'), 'rb'))


def run(*args):
    r = subprocess.run([sys.executable, '-m', 'pcbmill', *args], cwd=ROOT,
                       capture_output=True, text=True)
    if r.returncode:
        sys.exit('pcbmill %s failed:\n%s' % (' '.join(args), r.stderr))
    return r.stdout


def moves(path):
    """(mode, x, y, z) after every motion line."""
    x = y = 0.0
    z = CFG['machine']['safe_z']
    mode = 0
    out = []
    for line in open(path):
        ws = level.words(line)
        for l, v in ws:
            if l == 'G' and v in (0, 1):
                mode = int(v)
        t = {l: v for l, v in ws if l in 'XYZ'}
        if t and not any(l == 'G' and v == 4 for l, v in ws):
            x, y, z = t.get('X', x), t.get('Y', y), t.get('Z', z)
            out.append((mode, x, y, z))
    return out


def check_tap(path, depths, surface=lambda x, y: 0.0, tol=0.006):
    """No rapid below the surface; every cut sits at one of the expected depths."""
    errs = 0
    prev = None
    for mode, x, y, z in moves(path):
        rel = z - surface(x, y)
        if mode == 0 and rel < 0.5 - 1e-6 and prev and (prev[1], prev[2]) != (x, y):
            errs += 1
        if mode == 1 and rel < -1e-6 and not any(abs(rel + d) < tol for d in depths):
            errs += 1
        prev = (mode, x, y, z)
    return errs


def check_board(src):
    tmp = tempfile.mkdtemp(prefix='pcbmill-')
    for f in bd.cam_files(src):
        shutil.copy(f, tmp)
    print(run('cam', tmp, '--side', 'both').rstrip())
    out = os.path.join(tmp, 'cnc')
    taps = sorted(glob.glob(os.path.join(out, '*.tap')))
    t = CFG['board']['thickness']
    cut = CFG['cutout']
    want = {
        'iso': [CFG['isolation']['depth']],
        'drill': [CFG['drill']['depth']] if CFG['drill']['mode'] == 'spot' else
                 [k * CFG['drill']['peck'] for k in range(1, 20)] + [t + 0.3],
        'cut': [cut['depth'] * (i + 1) / math.ceil(cut['depth'] / cut['stepdown'] - 1e-9)
                for i in range(math.ceil(cut['depth'] / cut['stepdown'] - 1e-9))],
    }
    bad = 0
    for p in taps:
        kind = re.search(r'_(iso|drill|cut)\.tap$', p).group(1)
        e = check_tap(p, want[kind])
        bad += e
        print('  %-40s %s' % (os.path.basename(p), 'ok' if not e else '%d ошибок' % e))

    # mirrored bottom side: every plated round hole must land on bottom copper
    raw = bd.load(tmp)
    if 'bottom' in raw.copper and raw.holes:
        b = bd.transform(raw, mirror=True)
        from shapely.geometry import Point
        cu = b.copper['bottom']
        holes = [h for h in b.holes if h.x2 is None and h.plated is not False]
        miss = sum(not cu.buffer(0.01).contains(Point(h.x, h.y)) for h in holes)
        print('  зеркало низа: %d из %d отверстий вне меди' % (miss, len(holes)))
        bad += miss > len(holes) * 0.1

    # levelling: fake warped board, probe log, map, levelled iso must follow it
    iso = [p for p in taps if p.endswith('_iso.tap')][0]
    probe = os.path.join(tmp, 'probe.nc')
    run('probe', iso, '-o', probe)
    pts = [tuple(map(float, re.findall(r'[XY](-?[\d.]+)', l)))
           for l in open(probe) if l.startswith('G0 X')]
    f = lambda x, y: 0.003 * x - 0.002 * y + 0.15 * math.sin(x / 70 * 3.1) * math.sin(y / 60 * 3.1)
    log = os.path.join(tmp, 'grbl.log')
    with open(log, 'w') as fh:
        for x, y in pts[:-1]:
            fh.write('[PRB:%.3f,%.3f,%.3f:1]\nok\n' % (x - 200, y - 150, f(x, y) - f(*pts[0]) - 30))
    hm = os.path.join(tmp, 'hm.csv')
    run('map', log, '--tap', iso, '-o', hm)
    run('apply', iso, hm, '-o', os.path.join(tmp, 'lev.tap'))
    e = check_tap(os.path.join(tmp, 'lev.tap'), want['iso'], lambda x, y: f(x, y) - f(*pts[0]), tol=0.012)
    print('  выравнивание по карте: %s' % ('ok' if not e else '%d ошибок' % e))
    bad += e
    shutil.rmtree(tmp)
    return bad


if __name__ == '__main__':
    total = sum(check_board(d) for d in sys.argv[1:])
    print('ИТОГ: %s' % ('всё ok' if not total else '%d ошибок' % total))
    sys.exit(1 if total else 0)
