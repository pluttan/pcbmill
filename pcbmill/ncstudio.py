"""Weihong NcStudio V5 back-end: no API and no probe G-code, so commands are typed
into its MDI window ("Implement processing command") as keystrokes, one short move
at a time. Z is never read back: every move is G91 relative and we keep the sum.
Stop/pause in NcStudio only act at the end of a line, so descents are made of
small separate steps and contact is checked between them."""
import ctypes
import math
import sys
import time


# ==========================
# ===  Windows keystrokes===
# ==========================

class WinKeys:
    """SendInput into the NcStudio window. Text goes as VK_PACKET unicode so the
    active keyboard layout (Russian included) does not turn 'G' into 'п'."""
    VK = {'ctrl': 0x11, 'shift': 0x10, 'alt': 0x12, 'enter': 0x0D, 'esc': 0x1B,
          **{'f%d' % i: 0x6F + i for i in range(1, 13)}}

    def __init__(self, title='NcStudio', mdi_key='ctrl+shift+f9'):
        if sys.platform != 'win32':
            raise RuntimeError('управление NcStudio работает только на Windows')
        from ctypes import wintypes
        self.u = ctypes.windll.user32
        self.title = title
        self.mdi_key = mdi_key
        self.hwnd = self._find()

        class KEYBDINPUT(ctypes.Structure):
            _fields_ = [('wVk', wintypes.WORD), ('wScan', wintypes.WORD), ('dwFlags', wintypes.DWORD),
                        ('time', wintypes.DWORD), ('dwExtraInfo', ctypes.c_size_t)]

        class INPUT(ctypes.Structure):
            class _U(ctypes.Union):
                _fields_ = [('ki', KEYBDINPUT), ('pad', ctypes.c_byte * 32)]
            _anonymous_ = ('u',)
            _fields_ = [('type', wintypes.DWORD), ('u', _U)]
        self.KEYBDINPUT, self.INPUT = KEYBDINPUT, INPUT

    def _find(self):
        found = []
        buf = ctypes.create_unicode_buffer(256)
        proto = ctypes.WINFUNCTYPE(ctypes.c_bool, ctypes.c_void_p, ctypes.c_void_p)
        def cb(h, _):
            if self.u.IsWindowVisible(h):
                self.u.GetWindowTextW(h, buf, 256)
                if self.title.lower() in buf.value.lower():
                    found.append(h)
            return True
        self.u.EnumWindows(proto(cb), 0)
        if not found:
            raise RuntimeError('окно NcStudio не найдено: запустите её')
        return found[0]

    def _send(self, vk=0, scan=0, flags=0):
        i = self.INPUT(type=1)
        i.ki = self.KEYBDINPUT(vk, scan, flags, 0, 0)
        self.u.SendInput(1, ctypes.byref(i), ctypes.sizeof(i))

    def focused(self):
        fg = self.u.GetForegroundWindow()
        # the MDI dialog is a child/owned window of NcStudio: compare root owners
        root = self.u.GetAncestor(fg, 3)
        return fg == self.hwnd or root == self.hwnd or self.u.GetWindow(fg, 4) == self.hwnd

    def focus(self):
        if not self.focused():
            self.u.SetForegroundWindow(self.hwnd)
            time.sleep(0.2)
        if not self.focused():
            raise RuntimeError('NcStudio не на переднем плане: не трогайте мышь и клавиатуру во время обхода')

    def combo(self, keys):
        vks = [self.VK[k] for k in keys.lower().split('+')]
        for v in vks:
            self._send(v)
        for v in reversed(vks):
            self._send(v, flags=2)

    def text(self, s):
        for ch in s:
            self._send(0, ord(ch), 4)        # KEYEVENTF_UNICODE
            self._send(0, ord(ch), 4 | 2)

    def open_mdi(self):
        self.focus()
        self.combo(self.mdi_key)
        time.sleep(0.4)

    def line(self, s):
        self.focus()
        self.text(s)
        self.combo('enter')


# ==========================
# ===  Machine interface ===
# ==========================

class NcStudio:
    """Same interface as machines.Grbl, plus stepped=True: the scan descends in steps."""
    stepped = True

    def __init__(self, keys, rapid=1500.0, settle=0.35, reopen=True, log=print):
        self.k = keys
        self.rapid = rapid             # G00 speed set in NcStudio, mm/min, for waiting
        self.settle = settle           # command overhead + acceleration, s
        self.reopen = reopen           # MDI window closes after Enter: reopen each time
        self.x = self.y = None         # unknown until the first absolute XY move
        self.z = 0.0                   # relative to where the tool was at start
        self.log = log
        self._mdi_open = False

    def _cmd(self, line, seconds):
        if self.reopen or not self._mdi_open:
            self.k.open_mdi()
            self._mdi_open = True
        self.k.line(line)
        time.sleep(seconds + self.settle)

    def position(self):
        return (self.x or 0.0, self.y or 0.0, self.z)

    def rapid_z(self, z):
        dz = z - self.z
        if abs(dz) < 1e-4:
            return
        self._cmd('G91 G00 Z%.4f' % dz, abs(dz) / self.rapid * 60)
        self.z = z

    def rapid_xy(self, x, y, feed=None):
        if self.x is None:
            dist = 300.0                # first move: position unknown, wait generously
        else:
            dist = math.hypot(x - self.x, y - self.y)
        self._cmd('G90 G00 X%.4f Y%.4f' % (x, y), dist / self.rapid * 60 + 0.3)
        self.x, self.y = x, y

    def move_z(self, z, feed):
        dz = z - self.z
        if abs(dz) < 1e-5:
            return
        self._cmd('G91 G01 Z%.4f F%.0f' % (dz, feed), abs(dz) / feed * 60)
        self.z = z

    def step_z(self, dz, feed):
        self.move_z(self.z + dz, feed)

    def wait_idle(self, timeout=60):
        pass                            # every command already waited for its own duration

    def stop(self):
        pass                            # steps are short and end on their own

    def set_z_zero_at(self, z):
        self.move_z(z, 20)
        input('Фреза касается меди. В NcStudio обнулите Z рабочей системы координат '
              '(кнопка «Z → 0» / «Clear Z»), затем нажмите Enter здесь… ')
        self.z = 0.0
        self.k.focus()
