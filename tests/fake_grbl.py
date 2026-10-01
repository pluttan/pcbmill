"""Serial-level GRBL 1.1 stand-in on top of SimMachine: line protocol with ok/error,
'?' status reports in MPos + WCO, $J jogs, 0x85 jog cancel, G10 L20 work offset."""
import re
import threading
import time

from pcbmill import sim


class FakeGrblSerial:
    def __init__(self, machine):
        self.m = machine
        self.out = bytearray()
        self.inbuf = b''
        self.lock = threading.Lock()
        self.wco = [0.0, 0.0, 0.0]          # machine = work + wco
        self.jogging = False
        self.lines = []

    # ---- pyserial surface ----
    def reset_input_buffer(self):
        with self.lock:
            self.out.clear()

    def read(self, n=1):
        end = time.time() + 0.1
        while time.time() < end:
            with self.lock:
                if self.out:
                    d = bytes(self.out[:n])
                    del self.out[:n]
                    return d
            time.sleep(0.001)
        return b''

    def write(self, data):
        for b in data:
            c = bytes([b])
            if c == b'?':
                self._status()
            elif c == b'\x85':
                if self.jogging:
                    self.m.stop()
            elif c in (b'\r',):
                continue
            elif c == b'\n':
                line, self.inbuf = self.inbuf.decode(), b''
                if line.strip():
                    self._line(line.strip())
            else:
                self.inbuf += c

    # ---- protocol ----
    def _emit(self, s):
        with self.lock:
            self.out += (s + '\r\n').encode()

    def _status(self):
        x, y, z = self.m.position()
        state = 'Idle' if self.m._idle.is_set() else ('Jog' if self.jogging else 'Run')
        if state == 'Idle':
            self.jogging = False
        mp = (x + self.wco[0], y + self.wco[1], z + self.wco[2])
        self._emit('<%s|MPos:%.3f,%.3f,%.3f|FS:0,0|WCO:%.3f,%.3f,%.3f>' % ((state,) + mp + tuple(self.wco)))

    def _line(self, line):
        self.lines.append(line)
        w = dict((k, float(v)) for k, v in re.findall(r'([XYZFLP])([-\d.]+)', line))
        x, y, z = self.m.position()
        if line.startswith('$J='):
            if 'F' not in w:
                return self._emit('error:22')
            self.jogging = True
            self.m._go(w.get('X', x), w.get('Y', y), w.get('Z', z), w['F'], False)
        elif 'G10' in line and w.get('L') == 20:
            # G10 L20 P1 Zv: current position becomes work Z = v
            dz = z - w['Z']
            self.wco[2] += dz
            self.m.z -= dz
            self.m.z_offset += dz
        elif 'G0' in line or 'G1' in line:
            feed = 1500 if 'G0' in line else w.get('F')
            if feed is None:
                return self._emit('error:22')
            self.m._go(w.get('X', x), w.get('Y', y), w.get('Z', z), feed, False)
        self._emit('ok')


def make():
    m = sim.SimMachine()
    return m, FakeGrblSerial(m)
