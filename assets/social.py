"""Generate the GitHub social preview (1280x640) with logo, title and an elevation profile.

Usage: python social.py [output.svg]   (default: social.svg next to this script)

Needs numpy and matplotlib (for the contour lines). The title and subtitle are the outlined
paths from logo-title-dark.svg, so the result does not depend on installed fonts.
Export to PNG manually, e.g. with: inkscape social.svg --export-type=png
"""

import math
import os
import random
import re
import sys

import matplotlib
import numpy as np

matplotlib.use("Agg")
import matplotlib.pyplot as plt  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
W, H = 1280, 640

YELLOW = "#f2c230"
FG, SUB = "#fff6ee", "#a9b6c3"

# Simplification tolerances in pixels (Ramer-Douglas-Peucker).
CONTOUR_TOL = 3.0
PROFILE_TOL = 0.5

src = open(os.path.join(HERE, "logo-title-dark.svg"), encoding="utf-8").read()
TITLE_D, SUB_D = re.findall(r'<path d="((?:M 176|m 150)[^"]+)"', src)
# bboxes (x, y, w, h) from `inkscape --query-all logo-title-dark.svg`
TITLE_BB = (140.969, 25.187, 238.438, 44.813)
SUB_BB = (142.34, 87.416, 361.716, 16.585)

LOGO = '''<rect width="100" height="100" rx="22" fill="url(#tile)" filter="url(#shadow)"/>
    <path d="M72.03 10.73C82.44 17.24 85.69 31.34 77.02 45.22C69.43 57.36 58.8 62.79 52.08 65.82L36.46 56.07C36.25 48.69 35.81 33.94 43.4 21.8C50.99 9.65 63.35 5.31 72.03 10.73ZM32.13 63.01L46.01 71.68C48.61 73.31 48.39 75.59 46.23 79.06L42.43 85.13C39.18 90.33 32.67 91.09 27.47 87.84L25.73 86.76C20.52 83.5 18.36 77.32 21.61 72.12L25.4 66.04C27.57 62.57 29.52 61.38 32.13 63.01Z" fill="#f2c230"/>
    <path d="M30.6 77.76C33.54 66.51 40.17 71.98 44.02 62.36C47.99 52.45 42.38 42.13 57.07 40.91C66.44 40.13 74.79 36.59 65.66 20.72" fill="none" stroke="url(#tile)" stroke-width="4.88" stroke-linecap="round"/>
    <circle cx="30.6" cy="77.76" r="4.85" fill="url(#tile)"/>
    <circle cx="65.66" cy="20.72" r="4.85" fill="url(#tile)"/>'''

DEFS = f'''<linearGradient id="tile" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="0" y2="100">
      <stop offset="0" stop-color="#305a84"/>
      <stop offset="1" stop-color="#1e3a56"/>
    </linearGradient>
    <filter id="shadow" x="-30%" y="-30%" width="160%" height="160%">
      <feGaussianBlur in="SourceAlpha" stdDeviation="4"/>
      <feOffset dy="3" result="b"/>
      <feFlood flood-color="#0a1520" flood-opacity="0.45"/>
      <feComposite in2="b" operator="in"/>
      <feMerge><feMergeNode/><feMergeNode in="SourceGraphic"/></feMerge>
    </filter>
    <linearGradient id="bg" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#22415f"/>
      <stop offset="1" stop-color="#132638"/>
    </linearGradient>
    <linearGradient id="prof" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="{YELLOW}" stop-opacity="0.2"/>
      <stop offset="1" stop-color="{YELLOW}" stop-opacity="0"/>
    </linearGradient>'''


def f(x):
    return f"{x:.1f}".rstrip("0").rstrip(".")


def rdp(pts, tol):
    """Ramer-Douglas-Peucker simplification of an (n, 2) array; keeps the end points."""
    keep = np.zeros(len(pts), dtype=bool)
    keep[0] = keep[-1] = True
    stack = [(0, len(pts) - 1)]
    while stack:
        a, b = stack.pop()
        if b - a < 2:
            continue
        p, q = pts[a], pts[b]
        seg = pts[a + 1:b]
        d = q - p
        n = math.hypot(*d)
        if n == 0:
            dist = np.hypot(*(seg - p).T)
        else:
            dist = np.abs(d[0] * (seg[:, 1] - p[1]) - d[1] * (seg[:, 0] - p[0])) / n
        i = int(np.argmax(dist))
        if dist[i] > tol:
            keep[a + 1 + i] = True
            stack += [(a, a + 1 + i), (a + 1 + i, b)]
    return pts[keep]


def catmull_rom(pts, closed):
    """Smooth cubic Bezier path through pts."""
    n = len(pts)

    def at(i):
        return pts[i % n] if closed else pts[min(max(i, 0), n - 1)]

    out = [f"M{f(pts[0][0])} {f(pts[0][1])}"]
    for i in range(n if closed else n - 1):
        p0, p1, p2, p3 = at(i - 1), at(i), at(i + 1), at(i + 2)
        c1 = p1 + (p2 - p0) / 6
        c2 = p2 - (p3 - p1) / 6
        out.append(f"C{f(c1[0])} {f(c1[1])} {f(c2[0])} {f(c2[1])} {f(p2[0])} {f(p2[1])}")
    return "".join(out) + ("Z" if closed else "")


def contours(peaks, seed, color, opacity, width=1.5, box=(0, 0, W, H), step=40):
    """Iso-lines of a single terrain field (max of distorted cones + gentle undulation), so lines never cross."""
    rnd = random.Random(seed)
    x0, y0, x1, y1 = box
    X, Y = np.meshgrid(np.arange(x0 - 40, x1 + 41, 8.0), np.arange(y0 - 40, y1 + 41, 8.0))
    Z = np.full(X.shape, -1e9)
    for (cx, cy, rmax) in peaks:
        harm = [(rnd.uniform(0.04, 0.12) / k ** 0.6, rnd.uniform(0, 2 * np.pi)) for k in range(2, 6)]
        dx, dy = (X - cx) / 1.25, Y - cy
        d, t = np.hypot(dx, dy), np.arctan2(dy, dx)
        m = 1 + sum(a * np.cos(k * t + ph + d / 300) for k, (a, ph) in zip(range(2, 6), harm))
        Z = np.maximum(Z, rmax - d / m)
    for _ in range(4):
        kx, ky, ph = rnd.uniform(-0.006, 0.006), rnd.uniform(-0.006, 0.006), rnd.uniform(0, 2 * np.pi)
        Z += 10 * np.sin(kx * X + ky * Y + ph)
    cs = plt.contour(X, Y, Z, levels=np.arange(step, Z.max(), step))
    res = []
    for segs in cs.allsegs:
        for seg in segs:
            if len(seg) < 3:
                continue
            closed = np.allclose(seg[0], seg[-1])
            pts = rdp(seg, CONTOUR_TOL)
            if closed:
                pts = pts[:-1]
            if len(pts) >= 3:
                res.append(f'<path d="{catmull_rom(pts, closed)}"/>')
    plt.close("all")
    return (f'<g fill="none" stroke="{color}" stroke-opacity="{opacity}" stroke-width="{width}">\n    '
            + "\n    ".join(res) + "\n  </g>")


def profile(course, seed, levels, amp=0.09, decay=0.62):
    """Piecewise-linear course plus fractal detail by midpoint displacement.

    The coarse levels are drawn first, so fewer levels give the same shape with less fine noise.
    """
    rnd = random.Random(seed)
    n = 2 ** levels
    ts = np.arange(n + 1) / n
    tc, ec = zip(*course)
    detail = np.zeros(n + 1)
    step = n
    while step > 1:
        half = step // 2
        for i in range(half, n, step):
            detail[i] = (detail[i - half] + detail[i + half]) / 2 + rnd.gauss(0, amp)
        step, amp = half, amp * decay
    return ts, np.interp(ts, tc, ec) + detail


def logo_at(x, y, size):
    return f'<g transform="translate({f(x)} {f(y)}) scale({size / 100})">\n    {LOGO}\n  </g>'


def placed(d, bb, cx, top, s, fill):
    """Path with bbox bb, horizontally centered at cx, top edge at top."""
    x = cx - (bb[0] + bb[2] / 2) * s
    y = top - bb[1] * s
    return f'<path transform="translate({f(x)} {f(y)}) scale({s})" d="{d}" fill="{fill}"/>'


def main():
    out = sys.argv[1] if len(sys.argv) > 1 else os.path.join(HERE, "social.svg")

    # Hike: valley start, climb, fore-summit, summit, shoulder, descent, counter-climb, valley end.
    course = [(0.0, 0.12), (0.06, 0.16), (0.16, 0.5), (0.21, 0.44), (0.27, 0.62), (0.31, 0.58),
              (0.42, 1.0), (0.47, 0.82), (0.53, 0.86), (0.66, 0.34), (0.72, 0.3), (0.83, 0.55),
              (0.88, 0.5), (1.0, 0.05)]
    ts, elev = profile(course, seed=3, levels=8)
    y_top, y_bot = 492, 590
    pts = np.column_stack([-20 + (W + 40) * ts, y_bot - (elev - elev.min()) / np.ptp(elev) * (y_bot - y_top)])
    pts = rdp(pts, PROFILE_TOL)
    line = "M" + "L".join(f"{f(x)} {f(y)}" for x, y in pts)
    area = line + f"L{W + 20} {H}L-20 {H}Z"

    ls, ts_, ss = 200, 1.75, 1.55  # logo size, title and subtitle scale
    top = 62
    body = f'''<rect width="{W}" height="{H}" fill="url(#bg)"/>
  {contours([(640, 250, 700)], 11, "#ffffff", 0.05)}
  <path d="{area}" fill="url(#prof)"/>
  <path d="{line}" fill="none" stroke="{YELLOW}" stroke-width="4" stroke-linejoin="round" stroke-opacity="0.9"/>
  {logo_at(640 - ls / 2, top, ls)}
  {placed(TITLE_D, TITLE_BB, 640, top + ls + 38, ts_, FG)}
  {placed(SUB_D, SUB_BB, 640, top + ls + 38 + TITLE_BB[3] * ts_ + 28, ss, SUB)}'''

    with open(out, "w", encoding="utf-8") as fh:
        fh.write(f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="{W}" height="{H}">
  <title>TRAMP – Track and Route Analysis, Mapping and Planning</title>
  <defs>
    {DEFS}
  </defs>
  {body}
</svg>
''')


if __name__ == "__main__":
    main()
