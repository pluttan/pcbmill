"""Keystroke sink standing in for NcStudio's MDI window: typed lines run on SimMachine.
drop: set of command numbers to ignore, like NcStudio does when a line arrives while busy."""
import re


class FakeKeys:
    def __init__(self, machine, drop=(), drop_if=None):
        self.m = machine
        self.drop = set(drop)
        self.drop_if = drop_if          # f(line) -> True to swallow it (once)
        self.n = 0
        self.lines = []
        self.opened = 0

    def focus(self):
        pass

    def open_mdi(self):
        self.opened += 1

    def line(self, s):
        self.n += 1
        self.lines.append(s)
        if self.n in self.drop:
            return
        if self.drop_if and self.drop_if(s):
            self.drop_if = None
            return
        w = dict((k, float(v)) for k, v in re.findall(r'([XYZF])([-\d.]+)', s))
        x, y, z = self.m.position()
        if 'G91' in s:
            tx, ty, tz = x + w.get('X', 0), y + w.get('Y', 0), z + w.get('Z', 0)
        else:
            tx, ty, tz = w.get('X', x), w.get('Y', y), w.get('Z', z)
        feed = 1500 if 'G00' in s else w['F']
        self.m._go(tx, ty, tz, feed, True)
