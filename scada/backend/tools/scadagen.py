"""Shared helpers for the generator scripts: SCADA tags from a simulator's
own signal list (go run ./cmd/<sim> -points-json), merging into
data/config.json, and drawing helpers for HMI views. Run the generator scripts
from scada/backend."""
import datetime
import json
import os
import subprocess

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..")

DEC = {"bar": 2, "l/s": 0, "NTU": 2, "mg/l": 2, "pH": 2, "m": 2, "m³": 0, "kW": 0, "MWh": 2, "%": 1,
       "Nm³/h": 0, "°C": 1, "mm/h": 1, "t/h": 1, "MW": 2, "Mvar": 2, "kV": 2, "Hz": 3, "A": 0, "rpm": 0}


def load_points(sim):
    """Signal list of a simulator directory under the repository root, e.g. 'blue-water'."""
    out = subprocess.run(["go", "run", f"./cmd/{sim}", "-points-json"], cwd=os.path.join(ROOT, sim),
                         check=True, capture_output=True, text=True, encoding="utf-8").stdout
    return json.loads(out)


def build_tags(points, dev, prefix):
    """SCADA tags for all points. Breakers and switches get a position tag
    (discrete input) and a command tag (coil, suffix .Cmd)."""
    tags = []

    def tag(id, name, area, addr, dt="bool", unit="", dec=0, writable=False, log=False, state=None, group="", ioa=0):
        t = dict(id=prefix + id, name=name, description=group, device=dev, area=area, address=addr, dataType=dt,
                 bit=-1, wordSwap=False, byteSwap=False, scale=1, offset=0, unit=unit, decimals=dec,
                 writable=writable, logEvents=log)
        if state:
            t["stateText"] = state
        if ioa:
            t["ioa"] = ioa
        tags.append(t)
        return t

    for p in points:
        k, a, g, ioa = p["kind"], p["modbus"], p["group"], p.get("ioa", 0)
        texts = {"0": p["offText"].capitalize(), "1": p["onText"].capitalize()}
        if k in ("breaker", "switch"):
            state = {"0": "Från", "1": "Till"} if k == "breaker" else texts
            t = tag(p["id"], p["desc"], "discrete", a, log=True, state=state, group=g, ioa=ioa)
            if p.get("ctrlIoa"):
                t["ctrlIoa"], t["cmd"] = p["ctrlIoa"], "C_DC" if k == "breaker" else "C_SC"
            tag(p["id"] + ".Cmd", p["desc"] + " – manöver", "coil", a, writable=True, group=g)
        elif k == "pulse":
            tag(p["id"], p["desc"], "coil", a, writable=True, group=g)
        elif k == "status":
            tag(p["id"], p["desc"], "discrete", a, log=True, state=texts, group=g, ioa=ioa)
        elif k in ("measurement", "step"):
            tag(p["id"], p["desc"], "input", a, "float32", p["unit"], DEC.get(p["unit"], 1), group=g, ioa=ioa)
        elif k == "setpoint":
            tag(p["id"], p["desc"], "holding", a, "float32", p["unit"], DEC.get(p["unit"], 1), writable=True, group=g, ioa=ioa)
    return tags


def merge_config(device, tags, path="data/config.json"):
    """Replace this device's tags in config.json. An existing device entry
    keeps its settings (host, port, ...) so local edits survive; other devices
    are left untouched."""
    cfg = json.load(open(path, encoding="utf-8"))
    old = next((d for d in cfg["devices"] if d["id"] == device["id"]), None)
    cfg["devices"] = [d for d in cfg["devices"] if d["id"] != device["id"]] + [{**device, **(old or {})}]
    cfg["tags"] = [t for t in cfg["tags"] if t["device"] != device["id"]] + tags
    json.dump(cfg, open(path, "w", encoding="utf-8"), ensure_ascii=False, indent=2)


FG, DIM, MUTED, PANEL = "#e6edf3", "#5c6b7a", "#8b949e", "#30363d"
WATER, DIRTY, AIR, GAS, GREEN, RED = "#4dabf7", "#a9845c", "#c5d6e8", "#ffd43b", "#2ecc71", "#ff4d4d"
NODYN = dict(dynTag="", dynRules=[])


class View:
    """Builds the element list of an HMI view. Tag ids are given without the
    device prefix."""

    def __init__(self, vid, name, w, h, prefix, idprefix):
        self.vid, self.name, self.w, self.h = vid, name, w, h
        self.prefix, self.idprefix = prefix, idprefix
        self.els, self.seq = [], 0

    def T(self, id):
        return self.prefix + id

    def add(self, type, x, y, w, h, **props):
        self.seq += 1
        self.els.append(dict(id=f"{self.idprefix}-{type}-{self.seq}", type=type, x=x, y=y, w=w, h=h, rot=0, props=props))

    def dyn(self, tag, on, off=DIM, op=">", value=0.5):
        other = {">": "<=", "==": "!="}[op]
        return dict(dynTag=self.T(tag), dynRules=[{"op": op, "value": value, "color": on},
                                                  {"op": other, "value": value, "color": off}])

    # ---- primitives ----
    def line(self, x, y, dx, dy, w=4, color=FG, d=None, dash=""):
        self.add("line", x, y, dx, dy, stroke=color, strokeWidth=w, dash=dash, **(d or NODYN))

    def text(self, x, y, w, s, size=14, color=MUTED, bold=False, align="start", h=24):
        self.add("text", x, y, w, h, text=s, fontSize=size, color=color, bold=bold, align=align, **NODYN)

    def rect(self, x, y, w, h, fill="#161b22", stroke=FG, sw=2, radius=4, d=None):
        self.add("rect", x, y, w, h, fill=fill, stroke=stroke, strokeWidth=sw, radius=radius, dash="", **(d or NODYN))

    def panel(self, x, y, w, h, title):
        self.add("rect", x, y, w, h, fill="#00000000", stroke=PANEL, strokeWidth=1, radius=6, dash="", **NODYN)
        self.text(x + 12, y + 6, w - 24, title, 14, FG, True)

    def m(self, x, y, tag, label, w=226, lo="", hi=""):
        self.add("measurement", x, y, w, 28, tag=self.T(tag), label=label, unit="", decimals="", fontSize=15, color=FG,
                 background="#0d1117", border=PANEL, lowLimit=lo, highLimit=hi, alarmColor=RED)

    def bar(self, x, y, w, h, tag, lo, hi, color=WATER):
        self.add("bar", x, y, w, h, tag=self.T(tag), min=lo, max=hi, color=color, background="#0d1117", showValue=False)

    def ind(self, x, y, tag, label, on_color=RED, on_value=1, blink=True):
        self.add("indicator", x, y, 18, 18, tag=self.T(tag), onValue=on_value, onColor=on_color, offColor="#3a4450",
                 label=label, blink=blink)

    def button(self, x, y, w, label, tag, ask=False, color=PANEL):
        self.add("button", x, y, w, 28, text=label, controlTag=self.T(tag), value=1, askValue=ask, color=color, textColor=FG)

    def setpoint(self, x, y, tag, label, w=226):
        self.m(x, y, tag, label, w - 68)
        self.button(x + w - 62, y, 62, "Ändra", tag, ask=True)

    def link(self, x, y, w, text, view):
        self.add("link", x, y, w, 32, text=text, view=view, color=PANEL, textColor=FG)

    def form(self, x, y, title, w, items, h=None, row=32):
        """Panel with one row per item: ("m", tag, label[, lo, hi]),
        ("sp", tag, label), ("t", tag, label, on, off[, color]),
        ("i", tag, label) or ("b", label, tag)."""
        cy = y + 36
        for kind, *a in items:
            if kind == "m":
                self.m(x + 12, cy, a[0], a[1], w - 24, *a[2:])
            elif kind == "sp":
                self.setpoint(x + 12, cy, *a, w=w - 24)
            elif kind == "t":
                self.toggle(x + 12, cy, a[0], a[1], a[2], a[3], w - 24, *a[4:])
            elif kind == "i":
                self.ind(x + 14, cy + 5, *a)
            elif kind == "b":
                self.button(x + 12, cy, w - 24, *a)
            cy += row
        self.panel(x, y, w, h or (cy - y + 8), title)
        return cy

    # ---- symbols ----
    def _onoff(self, type, x, y, w, h, label, status, control, on, off, con, coff, show=True):
        self.add(type, x, y, w, h, label=label, showLabel=show, statusTag=self.T(status), openValue=0, closedValue=1,
                 controlTag=self.T(control) if control else "", openCmd=0, closeCmd=1, offText=off, onText=on,
                 colorClosed=con, colorOpen=coff)

    def toggle(self, x, y, tag, label, on, off, w=226, color="#3d8bfd"):
        self._onoff("toggle", x, y, w, 28, label, tag, tag + ".Cmd", on, off, color, "#3a4450")

    def pump(self, x, y, name, size=40, label=None):
        """Pump symbol showing <name>.Running, operated through <name>.Run."""
        self._onoff("pump", x, y, size, size, label or name, name + ".Running", name + ".Run.Cmd", "START", "STOPP", GREEN, DIM)

    def valve(self, x, y, tag, label):
        self._onoff("valve", x, y, 40, 24, label, tag, tag + ".Cmd", "ÖPPNA", "STÄNG", GREEN, RED)

    def breaker(self, x, y, tag, label=""):
        self.add("breaker", x, y, 40, 40, label=label, showLabel=bool(label), statusTag=self.T(tag), openValue=0,
                 closedValue=1, controlTag=self.T(tag + ".Cmd"), openCmd=0, closeCmd=1, colorClosed=GREEN, colorOpen=GREEN)

    def save(self, path=None):
        path = path or f"data/views/{self.vid}.json"
        view = dict(id=self.vid, name=self.name, width=self.w, height=self.h, background="#0d1117", grid=10,
                    elements=self.els, updated=datetime.datetime.now().astimezone().isoformat())
        json.dump(view, open(path, "w", encoding="utf-8"), ensure_ascii=False, indent=2)
        return len(self.els)
