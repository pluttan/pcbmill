"""Height scan without a probe input: an MCU watches bit-to-copper contact,
this program moves the machine and stops it the moment contact is reported.

Runs on the PC that drives the machine (Windows 7 included), so it stays
Python 3.8 compatible and needs only pyserial (+ pywin32 for Mach3).

  python -m pcbmill.scan touch  --mcu COM5                 wiring check
  python -m pcbmill.scan grid   --mcu COM5 --tap board_iso.tap  -o scan.csv
  python -m pcbmill.scan zero   --mcu COM5 --tap board_iso.tap  (after tool change)
"""
import argparse
import math
import sys
import threading
import time

from . import level


class ScanError(Exception):
    pass


# ==========================
# ===  Touch sensor (MCU)===
# ==========================

class Touch:
    """Reads "T1"/"T0" events from the MCU in a thread; on contact calls the
    armed callback right from that thread so the stop is not delayed."""

    def __init__(self, port):
        self.port = port
        self.state = None
        self.hits = 0                   # rising edges seen, to catch contact during travel
        self.changed = threading.Event()
        self._cb = None
        self._lock = threading.Lock()
        self._alive = True
        self._t = threading.Thread(target=self._run, daemon=True)
        self._t.start()
        self.port.write(b'P?')
        deadline = time.time() + 3
        while self.state is None and time.time() < deadline:
            time.sleep(0.01)
        if self.state is None:
            raise ScanError('МК молчит: проверьте порт и прошивку firmware/touch')

    def _run(self):
        buf = b''
        while self._alive:
            data = self.port.read(64)
            if not data:
                continue
            buf += data
            while b'\n' in buf:
                line, buf = buf.split(b'\n', 1)
                line = line.strip()
                if line in (b'T1', b'S1'):
                    self._set(True)
                elif line in (b'T0', b'S0'):
                    self._set(False)

    def _set(self, v):
        with self._lock:
            old, self.state = self.state, v
            if v and not old:
                self.hits += 1
            cb = self._cb if v and not old else None
            if cb:
                self._cb = None
        if cb:
            cb()
        self.changed.set()

    def arm(self, cb):
        with self._lock:
            self._cb = cb

    def disarm(self):
        with self._lock:
            self._cb = None

    def close(self):
        self._alive = False


# ==========================
# ===  Scan procedure    ===
# ==========================

class Scanner:
    def __init__(self, machine, touch, clear=1.0, search=3.0, backoff=0.1,
                 coarse=20.0, fine=3.0, travel=600.0, log=print,
                 step_coarse=0.05, step_fine=0.005):
        self.m, self.t = machine, touch
        self.clear, self.search, self.backoff = clear, search, backoff
        self.coarse, self.fine, self.travel = coarse, fine, travel
        self.log = log
        self.step_coarse, self.step_fine = step_coarse, step_fine
        self.floor = None              # never go below this, work Z

    def _descend(self, depth, feed):
        """Move down until contact or depth; returns Z at stop or None."""
        z0 = self.m.position()[2]
        target = z0 - depth
        if self.floor is not None:
            target = max(target, self.floor)
        hit = []
        def stop():
            hit.append(1)
            self.m.stop()
        self.t.arm(stop)
        try:
            if self.t.state:          # contact appeared between checks
                stop()
            else:
                self.m.feed_to_z(target, feed)
            self.m.wait_idle(timeout=depth / feed * 60 * 2 + 10)
        finally:
            self.t.disarm()
        if not hit and self.t.state:  # event raced the end of the move
            hit.append(1)
        return self.m.position()[2] if hit else None

    def _step_down(self, depth, step, feed):
        """Stepped machines (NcStudio): small separate moves, contact checked between them.
        The tip ends at most one step below the copper."""
        z0 = self.m.position()[2]
        n = int(math.ceil(depth / step - 1e-9))
        for k in range(1, n + 1):
            z = z0 - min(depth, k * step)
            if self.floor is not None and z < self.floor:
                return None
            self.m.move_z(z, feed)
            if self.t.state:
                return z
        return None

    def _travel(self, fn, *args):
        """A move that must never touch copper; a contact means a lost or ignored command."""
        h = self.t.hits
        fn(*args)
        if self.t.hits != h or self.t.state:
            raise ScanError('касание во время переезда: станок не выполнил подъём '
                            '(команда пропущена?) или заготовка выше ожидаемого')

    def _lift_until_open(self, z, step=None):
        step = step or self.backoff
        for k in range(1, 6):
            self.m.move_z(z + step * k, self.fine * 10)
            time.sleep(0.05)
            if not self.t.state:
                return
        raise ScanError('контакт не размыкается после подъёма на %.2f мм: зажим замкнут на фрезу?'
                        % (step * 5))

    def touch_at(self, x, y, z_clear):
        """Touch copper at (x, y), return work Z of the surface."""
        if self.t.state:
            raise ScanError('контакт до переезда: фрезу не подняло')
        self._travel(self.m.rapid_z, z_clear)
        self._travel(self.m.rapid_xy, x, y, self.travel)
        if self.t.state:
            raise ScanError('контакт есть ещё над платой (X%.2f Y%.2f): провод замкнут или фреза '
                            'стоит на меди' % (x, y))
        stepped = getattr(self.m, 'stepped', False)
        if stepped:
            z = self._step_down(self.search, self.step_coarse, self.coarse)
        else:
            z = self._descend(self.search, self.coarse)
        if z is None:
            self.m.rapid_z(z_clear)
            raise ScanError('медь не найдена на %.2f мм ниже Z%.2f в X%.2f Y%.2f: зажим отвалился, '
                            'грязная медь или край заготовки' % (self.search, z_clear, x, y))
        self._lift_until_open(z, self.step_coarse if stepped else None)
        if stepped:
            zf = self._step_down(self.backoff * 6, self.step_fine, self.fine * 10)
        else:
            zf = self._descend(self.backoff * 6, self.fine)
        if zf is None:
            raise ScanError('повторное касание не случилось в X%.2f Y%.2f' % (x, y))
        self._lift_until_open(zf, self.step_coarse if stepped else None)
        self._travel(self.m.rapid_z, zf + self.clear)
        return zf

    def grid(self, pts, out=None):
        z_clear = self.m.position()[2]
        self.floor = z_clear - self.search - 1.0
        rows = []
        t0 = time.time()
        try:
            for i, (x, y) in enumerate(pts):
                z = self.touch_at(x, y, z_clear)
                if rows and z > max(r[2] for r in rows) + self.clear * 0.6:
                    # copper right under the lift height: either a 0.6 mm bump between
                    # neighbours or the machine skipped our lift and the count is off
                    raise ScanError('в X%.2f Y%.2f медь на %.3f мм выше всех прежних точек: '
                                    'похоже, станок пропустил команду подъёма. Увеличьте --settle'
                                    % (x, y, z - max(r[2] for r in rows)))
                rows.append((x, y, z))
                if out:
                    out.write('%.3f,%.3f,%.4f\n' % (x, y, z))
                    out.flush()
                # next approach starts just above the copper found so far
                z_clear = max(r[2] for r in rows) + self.clear
                self.floor = min(r[2] for r in rows) - self.search
                left = (time.time() - t0) / (i + 1) * (len(pts) - i - 1)
                self.log('%3d/%d  X%7.2f Y%7.2f  Z%8.4f   осталось ~%.0f мин'
                         % (i + 1, len(pts), x, y, z, left / 60))
        except BaseException:
            self.m.stop()
            try:
                self.m.wait_idle(5)
                self.m.rapid_z(self.m.position()[2] + self.clear)
            except Exception:
                pass
            raise
        return rows


# ==========================
# ===  Command line      ===
# ==========================

def open_serial(name, baud=115200, timeout=0.05):
    import serial
    return serial.Serial(name, baud, timeout=timeout)


def make_machine(a):
    from . import machines
    if a.machine == 'grbl':
        return machines.Grbl(open_serial(a.port, a.baud, 0.1))
    if a.machine == 'mach3':
        return machines.Mach3()
    if a.machine == 'ncstudio':
        from . import ncstudio
        keys = ncstudio.WinKeys(a.title, a.mdi_key)
        return ncstudio.NcStudio(keys, rapid=a.rapid, settle=a.settle, reopen=not a.mdi_stays)
    if a.machine == 'sim':
        from . import sim
        return sim.SimMachine()
    raise ScanError('неизвестный станок %s' % a.machine)


def make_touch(a, machine):
    if a.machine == 'sim':
        from . import sim
        return sim.SimTouch(machine)
    return Touch(open_serial(a.mcu))


def grid_from(a):
    if a.tap:
        with open(a.tap) as f:
            b = level.cut_bounds(f.readlines())
    else:
        x0, y0, x1, y1 = (float(v) for v in a.area.split(','))
        b = (x0, x1, y0, y1)
    return level.grid_points(b, a.grid, a.margin)[2]


def cmd_touch(a):
    m = make_machine(a) if a.machine == 'sim' else None
    t = make_touch(a, m)
    print('контакт сейчас: %s. Касайтесь зажимом фрезы, Ctrl+C для выхода' % ('ЕСТЬ' if t.state else 'нет'))
    last = t.state
    n = 0
    try:
        while True:
            t.changed.wait(0.5)
            t.changed.clear()
            if t.state != last:
                last = t.state
                n += 1
                print('%s  %s' % (time.strftime('%H:%M:%S'), 'КОНТАКТ' if last else 'разомкнуто'))
    except KeyboardInterrupt:
        print('переключений: %d' % n)


def scanner(a, m, t):
    return Scanner(m, t, clear=a.clear, search=a.search, coarse=a.coarse, fine=a.fine,
                   step_coarse=a.step, step_fine=a.step_fine)


def cmd_grid(a):
    pts = grid_from(a)
    m = make_machine(a)
    t = make_touch(a, m)
    print('%d точек; фреза должна стоять над первой точкой на 1–3 мм над медью, шпиндель выключен'
          % len(pts))
    if not a.yes and input('начать? [y/N] ').strip().lower() != 'y':
        return
    with open(a.out, 'w') as f:
        f.write('# x,y,z  pcbmill scan, Z в рабочих координатах\n')
        rows = scanner(a, m, t).grid(pts, f)
    zs = [r[2] for r in rows]
    print('готово: %s, перепад %.3f мм' % (a.out, max(zs) - min(zs)))
    print('дальше: pcbmill map %s --tap ... и после смены фрезы scan zero' % a.out)


def cmd_zero(a):
    """After a tool change: touch once at the first grid point and make it Z0,
    which is exactly the reference the height map is relative to."""
    if a.at:
        x, y = (float(v) for v in a.at.split(','))
    else:
        x, y = grid_from(a)[0]
    m = make_machine(a)
    t = make_touch(a, m)
    print('касание в X%.3f Y%.3f подачей %g мм/мин; фреза на 1–3 мм над медью, шпиндель выключен'
          % (x, y, a.fine))
    if not a.yes and input('начать? [y/N] ').strip().lower() != 'y':
        return
    s = scanner(a, m, t)
    s.coarse = min(a.coarse, 10.0)     # the thin tip goes in slowly
    z = s.touch_at(x, y, m.position()[2])
    m.set_z_zero_at(z)
    m.rapid_z(a.clear)
    print('Z0 выставлен на меди в X%.3f Y%.3f (касание было на Z%.4f)' % (x, y, z))


def cmd_nctest(a):
    """One MDI lift and return: proves keystrokes reach NcStudio before any scan."""
    m = make_machine(a)
    print('Фреза должна быть не ниже 5 мм над заготовкой. Сейчас NcStudio получит две команды:')
    print('  G91 G00 Z1   (вверх на 1 мм)   и   G91 G00 Z-1   (обратно)')
    if not a.yes and input('начать? [y/N] ').strip().lower() != 'y':
        return
    m.rapid_z(1.0)
    up = input('Z поднялся на 1 мм? [y/N] ').strip().lower() == 'y'
    m.rapid_z(0.0)
    down = input('Z вернулся? [y/N] ').strip().lower() == 'y'
    if up and down:
        print('нажатия доходят; если окно MDI осталось открытым, дальше можно с --mdi-stays')
    else:
        print('не сработало: проверьте --mdi-key (клавиши окна MDI в вашей NcStudio) и --title')


def cmd_level(a):
    """scan.csv + board program -> *_level.tap, on the machine PC itself."""
    hm, _, n = level.build_map(a.out)
    with open(a.tap) as f:
        text = level.apply_map(f.readlines(), hm, 1.0)
    out = a.tap.rsplit('.', 1)[0] + '_level.tap'
    with open(out, 'w') as f:
        f.write(text)
    print(hm.table())
    print('касаний %d -> %s' % (n, out))


def main(argv=None):
    p = argparse.ArgumentParser(prog='pcbmill.scan', description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    sp = p.add_subparsers(dest='cmd')
    for name, fn, h in (('touch', cmd_touch, 'проверка провода: печатает касания'),
                        ('grid', cmd_grid, 'обойти сетку и записать высоты'),
                        ('zero', cmd_zero, 'после смены фрезы: одно касание и Z0'),
                        ('nctest', cmd_nctest, 'ncstudio: проверить, что команды доходят'),
                        ('level', cmd_level, 'scan.csv + программа -> программа_level.tap')):
        g = sp.add_parser(name, help=h)
        g.add_argument('--mcu', help='порт МК, например COM5')
        g.add_argument('--machine', default='ncstudio', choices=['ncstudio', 'grbl', 'mach3', 'sim'])
        g.add_argument('--title', default='NcStudio', help='ncstudio: часть заголовка окна')
        g.add_argument('--mdi-key', default='ctrl+shift+f9', help='ncstudio: клавиши окна MDI')
        g.add_argument('--mdi-stays', action='store_true',
                       help='ncstudio: окно MDI не закрывается после Enter')
        g.add_argument('--rapid', type=float, default=1500, help='ncstudio: скорость G00, мм/мин')
        g.add_argument('--settle', type=float, default=0.35, help='ncstudio: запас на каждую команду, с')
        g.add_argument('--port', help='порт станка (GRBL)')
        g.add_argument('--baud', type=int, default=115200)
        g.add_argument('--tap', help='программа, по габариту которой строить сетку')
        g.add_argument('--area', help='вместо --tap: x0,y0,x1,y1')
        g.add_argument('--grid', type=float, default=10, help='шаг сетки, мм')
        g.add_argument('--step', type=float, default=0.05, help='ncstudio: шаг поиска по Z, мм')
        g.add_argument('--step-fine', type=float, default=0.005, help='ncstudio: шаг точного касания, мм')
        g.add_argument('--margin', type=float, default=0)
        g.add_argument('--at', help='zero: X,Y точки касания (по умолчанию первая точка сетки)')
        g.add_argument('--clear', type=float, default=1.0, help='подъём между точками, мм')
        g.add_argument('--search', type=float, default=3.0, help='на сколько вниз искать медь, мм')
        g.add_argument('--coarse', type=float, default=20, help='подача поиска, мм/мин')
        g.add_argument('--fine', type=float, default=3, help='подача точного касания, мм/мин')
        g.add_argument('-o', '--out', default='scan.csv')
        g.add_argument('-y', '--yes', action='store_true', help='не спрашивать подтверждение')
        g.set_defaults(fn=fn)
    a = p.parse_args(argv)
    if not a.cmd:
        p.print_help()
        return 1
    try:
        a.fn(a)
    except ScanError as e:
        print('ОСТАНОВ: %s' % e)
        return 2
    except KeyboardInterrupt:
        print('прервано, станок остановлен')
        return 3
    return 0


if __name__ == '__main__':
    sys.exit(main())
