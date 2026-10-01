"""Simulated machine and touch sensor: the scan logic is exercised against a
warped board with realistic stop latency, without any hardware."""
import math
import threading
import time


def warped(x, y):
    """Board 0.4 mm out of flat: tilt plus a bump, surface Z in work coordinates."""
    return 0.003 * x - 0.002 * y + 0.15 * math.sin(x / 70 * 3.1) * math.sin(y / 60 * 3.1) - 1.5


class SimMachine:
    def __init__(self, surface=warped, speedup=10.0, start=(0.0, 0.0, 0.0), decel=0.02):
        self.surface = surface
        self.speedup = speedup          # simulated seconds per real second
        self.decel = decel              # mm travelled after a stop request at full feed of 60
        self.x, self.y, self.z = start
        self.target = None
        self.feed = 0.0
        self.touch = None
        self.max_dig = 0.0              # deepest the tip went below the copper
        self.z_offset = 0.0
        self._lock = threading.Lock()
        self._stop = threading.Event()
        self._idle = threading.Event()
        self._idle.set()
        threading.Thread(target=self._run, daemon=True).start()

    # ---- motion thread ----
    def _run(self):
        dt = 0.001
        while True:
            time.sleep(dt)
            with self._lock:
                if self.target is not None:
                    tx, ty, tz = self.target
                    d = math.dist((self.x, self.y, self.z), (tx, ty, tz))
                    step = self.feed / 60 * dt * self.speedup
                    if self._stop.is_set():
                        # decelerate: a little more travel, proportional to feed
                        extra = min(d, self.decel * self.feed / 60)
                        k = extra / d if d else 0
                        self.x += (tx - self.x) * k
                        self.y += (ty - self.y) * k
                        self.z += (tz - self.z) * k
                        self.target = None
                    elif d <= step:
                        self.x, self.y, self.z = tx, ty, tz
                        self.target = None
                    else:
                        k = step / d
                        self.x += (tx - self.x) * k
                        self.y += (ty - self.y) * k
                        self.z += (tz - self.z) * k
                    if self.target is None:
                        self._stop.clear()
                        self._idle.set()
                surf = self.surface(self.x, self.y) - self.z_offset
                self.max_dig = max(self.max_dig, surf - self.z)
                contact = self.z <= surf
            if self.touch is not None:
                self.touch.sense(contact)

    def _go(self, x, y, z, feed, wait):
        with self._lock:
            self.target = (x, y, z)
            self.feed = feed
            self._idle.clear()
        if wait:
            self.wait_idle()

    # ---- machine interface ----
    def position(self):
        with self._lock:
            return self.x, self.y, self.z

    def rapid_z(self, z):
        x, y, _ = self.position()
        self._go(x, y, z, 1500, True)

    def rapid_xy(self, x, y, feed=1500):
        self._go(x, y, self.position()[2], 1500, True)

    def move_z(self, z, feed):
        x, y, _ = self.position()
        self._go(x, y, z, feed, True)

    def feed_to_z(self, z, feed):
        x, y, _ = self.position()
        self._go(x, y, z, feed, False)

    def stop(self):
        self._stop.set()

    def wait_idle(self, timeout=60):
        if not self._idle.wait(timeout / self.speedup + 5):
            raise TimeoutError('sim machine did not stop')

    def set_z_zero_at(self, z):
        with self._lock:
            self.z_offset += z
            self.z -= z


class SimTouch:
    """Touch sensor with MCU + USB + host latency (simulated seconds)."""

    def __init__(self, machine, latency=0.03, broken=False, shorted=False):
        self.m = machine
        self.latency = latency
        self.broken, self.shorted = broken, shorted
        self.state = shorted
        self.hits = 0
        self.changed = threading.Event()
        self._cb = None
        self._pending = None
        self._lock = threading.Lock()
        machine.touch = self

    def sense(self, contact):
        if self.broken:
            contact = False
        if self.shorted:
            contact = True
        now = time.time()
        with self._lock:
            if contact != self.state:
                if self._pending is None or self._pending[1] != contact:
                    self._pending = (now + self.latency / self.m.speedup, contact)
            else:
                self._pending = None
            fire = self._pending and now >= self._pending[0]
            cb = None
            if fire:
                self.state = self._pending[1]
                self._pending = None
                if self.state:
                    self.hits += 1
                if self.state and self._cb:
                    cb, self._cb = self._cb, None
        if fire:
            self.changed.set()
        if cb:
            cb()

    def arm(self, cb):
        with self._lock:
            self._cb = cb

    def disarm(self):
        with self._lock:
            self._cb = None

    def close(self):
        pass
