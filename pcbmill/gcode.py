"""Operation -> G-code text in the same flavour as the .tap files this machine already ran."""
import math


def n(v):
    return '%.3f' % (0.0 if abs(v) < 5e-4 else v)


def header(op, mcfg, tool, title):
    out = ['(%s: %s)' % (title, op.name),
           '(tool %s: %s)' % (op.tool, ', '.join('%s=%s' % kv for kv in tool.items()))]
    out += ['(%s)' % s for s in op.notes]
    out.append('G21G90G17G94')
    if mcfg['dialect'] == 'grbl':
        out.append('(T1M6)')          # grbl rejects M6; tool is changed by hand before the run
    else:
        out.append('T1M6')
    out.append('G0Z%s' % n(mcfg['safe_z']))
    out.append('G0X0.000Y0.000S%dM3' % mcfg['spindle'])
    if mcfg.get('spinup'):
        out.append('G4P%s' % mcfg['spinup'])
    return out


def footer(mcfg):
    px, py = mcfg.get('park', [0, 0])
    return ['G0Z%s' % n(mcfg['safe_z']), 'G0X%sY%s' % (n(px), n(py)), 'M5', 'M30']


def body(op, mcfg, tool):
    out = []
    tz = mcfg['travel_z']
    plunge, feed = tool['plunge'], tool['feed']
    x0 = y0 = None
    for p in op.paths:
        x, y, z = p.pts[0]
        if (x0, y0) != (x, y):
            out.append('G0Z%s' % n(tz))
            out.append('G0X%sY%s' % (n(x), n(y)))
        if p.peck:
            d = 0.0
            while d < -z - 1e-9:
                d = min(-z, d + p.peck)
                out.append('G1Z%sF%.1f' % (n(-d), plunge))
                out.append('G0Z%s' % n(0.5 if d < -z else tz))
        else:
            out.append('G1Z%sF%.1f' % (n(z), plunge))
            first = True
            lz = z
            for x, y, z in p.pts[1:]:
                s = 'X%sY%s' % (n(x), n(y))
                if abs(z - lz) > 1e-6:
                    s += 'Z%s' % n(z)
                    lz = z
                if first:
                    s = 'G1' + s + 'F%.1f' % feed
                    first = False
                out.append(s)
        x0, y0 = p.pts[-1][0], p.pts[-1][1]
    return out


def emit(op, mcfg, tool, title='pcbmill'):
    lines = header(op, mcfg, tool, title) + body(op, mcfg, tool) + footer(mcfg)
    return '\n'.join(lines) + '\n'


def stats(op, mcfg, tool):
    """Rough run time: cut length / feed + plunges; rapids ignored."""
    cut = sum(math.dist(a[:2], b[:2]) for p in op.paths for a, b in zip(p.pts, p.pts[1:]))
    plunges = sum(max(1, math.ceil(-p.pts[0][2] / p.peck)) if p.peck else 1 for p in op.paths)
    minutes = cut / tool['feed'] + plunges * 2 * mcfg['travel_z'] / tool['plunge']
    return cut, minutes
