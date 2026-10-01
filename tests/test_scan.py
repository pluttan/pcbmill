"""Scan logic against the simulated machine: accuracy, how far the tip digs in,
and that every fault ends with the tool lifted and a clear message.
Usage: python3.12 tests/test_scan.py
"""
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, ROOT)
from pcbmill import level, scan, sim  # noqa: E402

PTS = level.grid_points((0, 60, 0, 40), 10, 0)[2]
bad = 0


def report(name, ok, detail=''):
    global bad
    bad += not ok
    print('  %-46s %s %s' % (name, 'ok' if ok else 'ОШИБКА', detail))


def scanner(m, t, **kw):
    return scan.Scanner(m, t, log=lambda *_: None, **kw)


# ---- accuracy and penetration ----
m = sim.SimMachine()
t = sim.SimTouch(m)
rows = scanner(m, t).grid(PTS)
err = max(abs(z - sim.warped(x, y)) for x, y, z in rows)
report('точность высоты на %d точках' % len(rows), err < 0.005, 'макс. ошибка %.4f мм' % err)
report('фреза вдавливается в медь не глубже 0.03 мм', m.max_dig < 0.03, '%.4f мм' % m.max_dig)
report('после обхода фреза над медью', m.position()[2] > sim.warped(*m.position()[:2]) + 0.5)

# the map built from the scan follows the board
hm = level.HeightMap.from_points([(x, y, z - rows[0][2]) for x, y, z in rows], 0.05)
dev = max(abs(hm.at(x + 3, y + 4) - (sim.warped(x + 3, y + 4) - rows[0][2]))
          for x, y in PTS if x < 60 and y < 40)
report('карта между точками (интерполяция)', dev < 0.02, 'макс. отклонение %.4f мм' % dev)

# ---- zero after a tool change: Z0 lands on copper at the first point ----
m.set_z_zero_at(rows[0][2] + 0.0)
x0, y0 = PTS[0]
s = scanner(m, t, coarse=10)
z = s.touch_at(x0, y0, m.position()[2])
report('zero: повторное касание в первой точке = 0', abs(z) < 0.005, 'Z %.4f' % z)

# ---- zero with a different (thin) tool that sticks out 2.37 mm further ----
m2 = sim.SimMachine(start=(0, 0, 0))
t2 = sim.SimTouch(m2)
rows2 = scanner(m2, t2).grid(PTS[:4])
m2.set_z_zero_at(0)                       # nothing changed yet
m2.z_offset -= 2.37                       # new tool longer: copper appears 2.37 mm higher
m2.z += 0
s2 = scanner(m2, t2, coarse=10)
m2.rapid_z(3.0)
zt = s2.touch_at(PTS[0][0], PTS[0][1], 3.0)
m2.set_z_zero_at(zt)
zt2 = s2.touch_at(PTS[1][0], PTS[1][1], 2.0)
want = sim.warped(*PTS[1]) - sim.warped(*PTS[0])
report('смена фрезы: высоты после zero сходятся с картой', abs(zt2 - want) < 0.005,
       'Z %.4f, по карте %.4f' % (zt2, want))

# ---- faults ----
m = sim.SimMachine()
t = sim.SimTouch(m, broken=True)
try:
    scanner(m, t).grid(PTS)
    report('оторван зажим: останов', False, 'обход не остановился')
except scan.ScanError as e:
    x, y, z = m.position()
    report('оторван зажим: останов с подъёмом', z >= -0.01, str(e)[:60])
    report('оторван зажим: ниже допустимого не ушёл', m.max_dig <= 3.0 + 0.01,
           'глубже меди на %.2f' % m.max_dig)

m = sim.SimMachine()
t = sim.SimTouch(m, shorted=True)
try:
    scanner(m, t).grid(PTS)
    report('провод замкнут заранее: останов', False)
except scan.ScanError as e:
    report('провод замкнут заранее: останов без спуска', m.position()[2] >= -1e-6, str(e)[:60])

# slow MCU / host: 150 ms latency must still stop before 0.1 mm at the default feed
m = sim.SimMachine()
t = sim.SimTouch(m, latency=0.15)
scanner(m, t).grid(PTS[:6])
report('задержка 150 мс: вдавливание < 0.1 мм', m.max_dig < 0.1, '%.4f мм' % m.max_dig)

# ---- the real GRBL driver over a serial-level GRBL emulator ----
sys.path.insert(0, os.path.join(ROOT, 'tests'))
import fake_grbl  # noqa: E402
from pcbmill import machines  # noqa: E402
sm, port = fake_grbl.make()
g = machines.Grbl(port)
t = sim.SimTouch(sm)
rows = scanner(g, t).grid(PTS[:8])
err = max(abs(z - sim.warped(x, y)) for x, y, z in rows)
report('GRBL: точность через протокол', err < 0.005, 'макс. ошибка %.4f мм' % err)
report('GRBL: вдавливание', sm.max_dig < 0.03, '%.4f мм' % sm.max_dig)
report('GRBL: спуск только отменяемым $J', all(l.startswith('$J=') for l in port.lines
                                                if 'Z' in l and 'F20' in l), '')
z = scanner(g, t, coarse=10).touch_at(PTS[0][0], PTS[0][1], g.position()[2])
g.set_z_zero_at(z)
z2 = scanner(g, t, coarse=10).touch_at(PTS[0][0], PTS[0][1], 1.0)
report('GRBL: zero через G10 L20', abs(z2) < 0.005, 'повторное касание Z %.4f' % z2)

# ---- NcStudio: keystrokes into MDI, stepped descent, our own Z bookkeeping ----
import fake_ncstudio  # noqa: E402
from pcbmill import ncstudio  # noqa: E402
ncstudio.time.sleep = lambda s: None       # fake keys already block until the move ends
sm = sim.SimMachine()
keys = fake_ncstudio.FakeKeys(sm)
nc = ncstudio.NcStudio(keys, settle=0)
t = sim.SimTouch(sm)
rows = scanner(nc, t).grid(PTS[:12])
# NcStudio Z is relative to the start height: compare shapes, not absolute values
z0, s0 = rows[0][2], sim.warped(*PTS[0])
err = max(abs((z - z0) - (sim.warped(x, y) - s0)) for x, y, z in rows)
report('NcStudio: рельеф по шагам', err < 0.012, 'макс. ошибка %.4f мм' % err)
report('NcStudio: вдавливание не больше шага поиска', sm.max_dig <= 0.05 + 0.002, '%.4f мм' % sm.max_dig)
report('NcStudio: только G90 XY и G91 Z', all(re.match(r'G9[01] G0[01] ', l) for l in keys.lines), '')
per = len(keys.lines) / len(rows)
report('NcStudio: команд на точку', per < 40, '%.0f' % per)

# a swallowed lift must not turn into dragging the tip across copper
sm = sim.SimMachine()
lifts = []
def lift_after_touch(line):
    # the big G00 lift that ends the first point (after its fine steps)
    if line.startswith('G91 G00 Z') and float(line.split('Z')[1]) > 0.5 and lifts:
        return True
    if line.startswith('G91 G01'):
        lifts.append(line)
    return False
keys = fake_ncstudio.FakeKeys(sm, drop_if=lift_after_touch)
nc = ncstudio.NcStudio(keys, settle=0)
t = sim.SimTouch(sm)
try:
    scanner(nc, t).grid(PTS[:3])
    report('NcStudio: пропущенный подъём ловится', False, 'обход продолжился')
except scan.ScanError as e:
    report('NcStudio: пропущенный подъём ловится', 'переезд' in str(e) or 'пропустил' in str(e), str(e)[:55])
report('NcStudio: после пропуска фреза не ушла глубже шага', sm.max_dig <= 0.06, '%.4f мм' % sm.max_dig)

print('ИТОГ: %s' % ('всё ok' if not bad else '%d ошибок' % bad))
sys.exit(1 if bad else 0)
