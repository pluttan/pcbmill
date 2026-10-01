"""The machine must never get a program the checker would have stopped.

Two halves: every dangerous pattern below must be rejected, and every program
pcbmill itself produces (cam, probe, levelled, selftest, all dialects) must pass.
Usage: python3.12 tests/test_verify.py
"""
import copy
import glob
import os
import sys
import tempfile
import tomllib

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, ROOT)
from pcbmill import level, selftest, verify  # noqa: E402

CFG = tomllib.load(open(os.path.join(ROOT, 'machine.toml'), 'rb'))
OK_HEAD = ['G21G90G17G94', 'G0Z4', 'G0X0Y0S8000M3', 'G4P2']
OK_TAIL = ['G0Z4', 'G0X0Y0', 'M5', 'M30']


def prog(*body):
    return OK_HEAD + list(body) + OK_TAIL


def cfg_with(**kw):
    c = copy.deepcopy(CFG)
    for k, v in kw.items():
        sect, key = k.split('__')
        c[sect][key] = v
    return c


# (what the machine would do, program, kind, config) -> must be an error
DANGERS = [
    ('rapid sideways through copper',   prog('G0Z-0.1', 'G0X10'), 'iso', CFG),
    ('rapid plunge into material',      prog('G0X5Y5', 'G0Z-0.1'), 'iso', CFG),
    ('rapid sideways just above copper', prog('G0Z0.05', 'G0X10Y10'), 'iso', CFG),
    ('isolation 1 mm deep',             prog('G1Z-1F200', 'G1X10F300'), 'iso', CFG),
    ('drilling into the table',         prog('G0X5Y5', 'G1Z-3F200', 'G0Z4'), 'drill', CFG),
    ('inch mode',                       ['G20'] + prog('G1Z-0.1F200', 'G1X1F300'), 'iso', CFG),
    ('cut with spindle stopped',        ['G21G90', 'G0Z4', 'G0X0Y0', 'G1Z-0.1F200', 'G1X5F300',
                                         'G0Z4', 'M30'], 'iso', CFG),
    ('cut without feed rate',           ['G21G90', 'G0Z4', 'S8000M3', 'G1Z-0.1', 'G1X5', 'G0Z4',
                                         'M5', 'M30'], 'iso', CFG),
    ('M6 on GRBL',                      ['T1M6'] + prog(), 'iso', CFG),
    ('G31 probe on GRBL',               ['G21G90', 'G0Z2', 'G31Z-2F20', 'G0Z2', 'M2'], 'probe', CFG),
    ('G38.2 probe on Mach3',            ['G21G90', 'G0Z2', 'G38.2Z-2F20', 'G0Z2', 'M2'], 'probe',
                                        cfg_with(machine__dialect='mach3')),
    ('probe too fast for 0.1 tip',      ['G21G90', 'G0Z2', 'G38.2Z-2F300', 'G0Z2', 'M2'], 'probe', CFG),
    ('probe with spindle running',      ['G21G90', 'G0Z2', 'S8000M3', 'G38.2Z-2F20', 'G0Z2', 'M5',
                                         'M2'], 'probe', CFG),
    ('probe moving sideways',           ['G21G90', 'G0Z2', 'G38.2X5Z-2F20', 'G0Z2', 'M2'], 'probe', CFG),
    ('first move sideways, Z unknown',  ['G21G90', 'G0X10Y10', 'G0Z4', 'M2'], 'test', CFG),
    ('motion mode never set',           ['G21G90', 'X10Y10', 'M2'], 'test', CFG),
    ('beyond X travel',                 prog('G0X500'), 'iso', CFG),
    ('above Z travel',                  prog('G0Z60'), 'iso', CFG),
    ('feed beyond machine',             prog('G1Z-0.1F200', 'G1X10F5000'), 'iso', CFG),
    ('spindle beyond machine',          ['G21G90', 'G0Z4', 'S30000M3', 'M5', 'M30'], 'test', CFG),
    ('does not fit the blank',          prog('G0X120Y10', 'G1Z-0.1F200', 'G1X130F300'), 'iso',
                                        cfg_with(board__blank=[100, 80])),
    ('ends with tool in copper',        OK_HEAD + ['G1Z-0.1F200', 'G1X5F300', 'M5', 'M30'], 'iso', CFG),
    ('nested comment parentheses',      ['(a (b) c)'] + prog(), 'iso', CFG),
    ('unclosed comment',                ['(abc'] + prog(), 'iso', CFG),
    ('garbage in line',                 prog('G1X5Y5F300 Z?'), 'iso', CFG),
    ('unknown G code',                  prog('G5X1'), 'iso', CFG),
]


def test_dangers():
    bad = 0
    for name, lines, kind, cfg in DANGERS:
        r = verify.check(lines, cfg, name, kind)
        if r.ok:
            bad += 1
            print('  НЕ ПОЙМАНО: %s' % name)
    print('опасные программы: поймано %d из %d' % (len(DANGERS) - bad, len(DANGERS)))
    return bad


def test_ours():
    """Everything pcbmill emits passes for every dialect."""
    bad = 0
    tmp = tempfile.mkdtemp(prefix='pcbmill-verify-')
    for dialect in ('grbl', 'mach3', 'linuxcnc'):
        cfg = cfg_with(machine__dialect=dialect)
        for name, fn, kind in selftest.PROGRAMS:
            r = verify.check(fn(cfg), cfg, '%s/%s' % (dialect, name), kind)
            if not r.ok:
                bad += 1
                print(r.text())
        pts = level.grid_points((0, 50, 0, 40), 10, 0)[2]
        text = level.probe_program(pts, dialect, 1.5, 2, 30, 'p.txt').splitlines()
        r = verify.check(text, cfg, '%s/probe' % dialect, 'probe')
        if not r.ok:
            bad += 1
            print(r.text())
    # cam output on the sample boards, plain and levelled
    for d in sorted(glob.glob(os.path.join(ROOT, 'examples', 'gerbers', '*'))):
        out = os.path.join(tmp, os.path.basename(d))
        rc = os.system('cd %s && %s -m pcbmill cam %s -o %s --side both >/dev/null 2>&1'
                       % (ROOT, sys.executable, d, out))
        if rc:
            bad += 1
            print('  cam упал или выдал .BAD на %s' % d)
        for p in glob.glob(os.path.join(out, '*.tap')):
            r = verify.check_file(p, CFG, verify.guess_kind(p))
            if not r.ok:
                bad += 1
                print(r.text())
    print('свои программы: %s' % ('все годятся' if not bad else '%d с ошибками' % bad))
    return bad


if __name__ == '__main__':
    bad = test_dangers() + test_ours()
    print('ИТОГ: %s' % ('всё ok' if not bad else '%d ошибок' % bad))
    sys.exit(1 if bad else 0)
