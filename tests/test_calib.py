"""Calibration boards: the programs pass the checker, never cut deeper than the
deepest sample, the combs have exactly the clearances on their labels, and the
height map can be laid over them like over any other program.
Usage: python3.12 tests/test_calib.py
"""
import os
import re
import sys
import tempfile
import tomllib

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, ROOT)
from shapely.ops import unary_union  # noqa: E402

from pcbmill import calib, level, toolpath as tp, verify  # noqa: E402

CFG = tomllib.load(open(os.path.join(ROOT, 'machine.toml'), 'rb'))


def zs(lines):
    return [float(m) for l in lines for m in re.findall(r'Z(-?[\d.]+)', l)]


def main():
    bad = 0
    out = tempfile.mkdtemp(prefix='pcbmill-calib-')
    made = calib.write(CFG, out, 200, 24000)
    for path, w, h, minutes, ops in made:
        lines = open(path).read().splitlines()
        r = verify.check(lines, CFG, path, 'iso')
        if not r.ok:
            bad += 1
            print(r.text())
        low = min(zs(lines))
        if low < -0.15 - 1e-9:
            bad += 1
            print('%s: рез глубже 0.15: %.3f' % (path, low))
        if w > 70 or h > 50:
            bad += 1
            print('%s: заготовка %.0f x %.0f мм больше задуманной' % (path, w, h))
        hm = level.HeightMap([0, 40, 80], [0, 30, 60], [[0.0, 0.1, 0.2], [0.05, 0.1, 0.0], [0.1, 0.0, -0.1]])
        try:
            level.apply_map(lines, hm, 1.0)
        except Exception as e:
            bad += 1
            print('%s: карта не накладывается: %s' % (path, e))
        print('  %s: %.0f x %.0f мм, ~%.0f мин, самый глубокий рез %.3f' % (os.path.basename(path), w, h, minutes, low))

    speed = open(made[0][0]).read()
    rpms = re.findall(r'S(\d+)M3', speed)
    if rpms != ['24000', '20000', '16000', '12000', '8000']:
        bad += 1
        print('обороты по рядам: %s' % rpms)
    feeds = sorted({int(float(f)) for f in re.findall(r'G1X[^F\n]*F([\d.]+)', speed)})
    if feeds != [100, 200, 300, 450]:
        bad += 1
        print('подачи по столбцам: %s' % feeds)

    for g in [0.15, 0.2, 0.25, 0.3, 0.4, 0.5]:
        a, b, _ = calib.comb(g)
        if abs(a.distance(b) - g) > 1e-9 or len(tp.polys(unary_union([a, b]))) != 2:
            bad += 1
            print('гребёнка %.2f: зазор %.3f' % (g, a.distance(b)))
    print('калибровочные платы: %s' % ('всё ok' if not bad else '%d ошибок' % bad))
    return bad


if __name__ == '__main__':
    sys.exit(1 if main() else 0)
