"""Machine back-ends for the scan: what is needed is move, stop now, read Z.

Grbl  — USB serial, stop = jog cancel (0x85), position from status reports.
Mach3 — its COM automation object ("Mach4.Document" -> GetScriptDispatch), stop =
        Stop button, position from DROs. Needs Mach3 already running; 32-bit
        Python is the safe choice since Mach3.exe is 32-bit.
"""
import re
import threading
import time


class Grbl:
    def __init__(self, port):
        self.p = port
        self.lock = threading.Lock()
        self.wco = (0.0, 0.0, 0.0)
        self.p.write(b'\r\n\r\n')
        time.sleep(2)                  # boards with auto-reset reboot on open
        self.p.reset_input_buffer()
        st = self.status()
        if st[0].startswith('Alarm'):
            raise RuntimeError('GRBL в аварии (Alarm): разблокируйте $X или сделайте Home в своей программе')
        self.send('G21G90')

    def _readline(self, timeout=5):
        end = time.time() + timeout
        buf = b''
        while time.time() < end:
            c = self.p.read(1)
            if not c:
                continue
            if c == b'\n':
                return buf.decode(errors='replace').strip()
            buf += c
        raise TimeoutError('GRBL не отвечает')

    def send(self, line, timeout=10):
        with self.lock:
            self.p.write(line.encode() + b'\n')
            while True:
                r = self._readline(timeout)
                if r == 'ok':
                    return
                if r.startswith('error') or r.startswith('ALARM'):
                    raise RuntimeError('GRBL на «%s» ответил %s' % (line, r))

    def status(self):
        with self.lock:
            self.p.write(b'?')
            while True:
                r = self._readline()
                if r.startswith('<'):
                    break
        state = r[1:].split('|')[0]
        m = re.search(r'WCO:([-\d.]+),([-\d.]+),([-\d.]+)', r)
        if m:
            self.wco = tuple(float(v) for v in m.groups())
        w = re.search(r'WPos:([-\d.]+),([-\d.]+),([-\d.]+)', r)
        if w:
            pos = tuple(float(v) for v in w.groups())
        else:
            mp = tuple(float(v) for v in re.search(r'MPos:([-\d.]+),([-\d.]+),([-\d.]+)', r).groups())
            pos = tuple(a - b for a, b in zip(mp, self.wco))
        return state, pos

    def position(self):
        return self.status()[1]

    def wait_idle(self, timeout=60):
        end = time.time() + timeout
        while time.time() < end:
            st = self.status()[0]
            if st == 'Idle':
                return
            if st.startswith('Alarm'):
                raise RuntimeError('GRBL ушёл в аварию')
            time.sleep(0.02)
        raise TimeoutError('станок не остановился за %.0f с' % timeout)

    def rapid_z(self, z):
        self.send('G90G0Z%.4f' % z)
        self.wait_idle()

    def rapid_xy(self, x, y, feed=600):
        self.send('G90G0X%.4fY%.4f' % (x, y))
        self.wait_idle()

    def move_z(self, z, feed):
        self.send('G90G1Z%.4fF%.1f' % (z, feed))
        self.wait_idle()

    def feed_to_z(self, z, feed):
        # a jog, because only jogs can be cancelled without losing position
        self.send('$J=G90G21Z%.4fF%.1f' % (z, feed))

    def stop(self):
        self.p.write(b'\x85')           # jog cancel, realtime, no lock needed

    def set_z_zero_at(self, z):
        cur = self.position()[2]
        self.send('G10L20P1Z%.4f' % (cur - z))


class Mach3:
    BTN_STOP = 1003
    DRO_X, DRO_Y, DRO_Z = 800, 801, 802

    def __init__(self):
        self.s = self._script()
        self.s.Code('G21 G90')
        self.wait_idle()

    @staticmethod
    def _script():
        import win32com.client
        try:
            return win32com.client.GetActiveObject('Mach4.Document').GetScriptDispatch()
        except Exception:
            raise RuntimeError('Mach3 не найден: он должен быть уже запущен; если запущен, '
                               'нет ключей Mach4.Document в реестре')

    def position(self):
        return (self.s.GetOEMDRO(self.DRO_X), self.s.GetOEMDRO(self.DRO_Y), self.s.GetOEMDRO(self.DRO_Z))

    def wait_idle(self, timeout=60):
        end = time.time() + timeout
        while time.time() < end:
            if not self.s.IsMoving():
                return
            time.sleep(0.02)
        raise TimeoutError('станок не остановился за %.0f с' % timeout)

    def rapid_z(self, z):
        self.s.Code('G90 G0 Z%.4f' % z)
        self.wait_idle()

    def rapid_xy(self, x, y, feed=600):
        self.s.Code('G90 G0 X%.4f Y%.4f' % (x, y))
        self.wait_idle()

    def move_z(self, z, feed):
        self.s.Code('G90 G1 Z%.4f F%.1f' % (z, feed))
        self.wait_idle()

    def feed_to_z(self, z, feed):
        # Code() returns at once for a single line; the move continues while we watch the MCU
        self.s.Code('G90 G1 Z%.4f F%.1f' % (z, feed))

    def stop(self):
        # COM objects are apartment-bound; the touch thread needs its own handle
        import pythoncom
        pythoncom.CoInitialize()
        self._script().DoOEMButton(self.BTN_STOP)

    def set_z_zero_at(self, z):
        cur = self.position()[2]
        self.s.SetOEMDRO(self.DRO_Z, cur - z)
