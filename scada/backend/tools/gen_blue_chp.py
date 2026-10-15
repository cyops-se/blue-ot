"""Generates the blue-chp device, tags and HMI view. The tags are derived from
the signal list of the simulator itself (go run ./cmd/blue-chp -points-json),
so they always match its Modbus map. Run from scada/backend:

    python tools/gen_blue_chp.py
"""
import datetime
import json
import os
import subprocess

DEV = "blue-chp"
PREFIX = "CHP."  # SCADA tag ids are global; keep the plant's tags apart
CHP_DIR = os.path.join(os.path.dirname(__file__), "..", "..", "..", "blue-chp")

points = json.loads(subprocess.run(["go", "run", "./cmd/blue-chp", "-points-json"], cwd=CHP_DIR,
                                   check=True, capture_output=True, text=True, encoding="utf-8").stdout)

# ---------------------------------------------------------------- tags
DEC = {"bar": 1, "t/h": 1, "°C": 1, "%": 1, "MW": 2, "Mvar": 2, "kV": 2, "Hz": 3, "A": 0, "rpm": 0, "MWh": 1, "m³/h": 0}
tags = []


def tag(id, name, area, addr, dt="bool", unit="", dec=0, writable=False, log=False, state=None, group=""):
    t = dict(id=PREFIX + id, name=name, description=group, device=DEV, area=area, address=addr, dataType=dt, bit=-1,
             wordSwap=False, byteSwap=False, scale=1, offset=0, unit=unit, decimals=dec,
             writable=writable, logEvents=log)
    if state:
        t["stateText"] = state
    tags.append(t)
    return t


def set_iec(t, p, cmd):
    if p.get("ctrlIoa"):
        t["ctrlIoa"] = p["ctrlIoa"]
        t["cmd"] = cmd


for p in points:
    k, a, g = p["kind"], p["modbus"], p["group"]
    texts = {"0": p["offText"].capitalize(), "1": p["onText"].capitalize()}
    if k == "breaker":
        set_iec(tag(p["id"], p["desc"], "discrete", a, log=True, state={"0": "Från", "1": "Till"}, group=g), p, "C_DC")
        tag(p["id"] + ".Cmd", p["desc"] + " – manöver", "coil", a, writable=True, group=g)
    elif k == "switch":
        set_iec(tag(p["id"], p["desc"], "discrete", a, log=True, state=texts, group=g), p, "C_SC")
        tag(p["id"] + ".Cmd", p["desc"] + " – manöver", "coil", a, writable=True, group=g)
    elif k == "pulse":
        tag(p["id"], p["desc"], "coil", a, writable=True, group=g)
    elif k == "status":
        tag(p["id"], p["desc"], "discrete", a, log=True, state=texts, group=g)
    elif k in ("measurement", "step"):
        tag(p["id"], p["desc"], "input", a, "float32", p["unit"], DEC.get(p["unit"], 0), group=g)
    elif k == "setpoint":
        tag(p["id"], p["desc"], "holding", a, "float32", p["unit"], DEC.get(p["unit"], 1), writable=True, group=g)

_mon = {p["id"]: p.get("ioa", 0) for p in points}
for _t in tags:
    _pid = _t["id"][len(PREFIX):] if _t["id"].startswith(PREFIX) else _t["id"]
    _ioa = _mon.get(_pid, 0)
    if _ioa and not _t["id"].endswith(".Cmd"):
        _t["ioa"] = _ioa

cfg = json.load(open("data/config.json", encoding="utf-8"))
cfg["devices"] = [d for d in cfg["devices"] if d["id"] != DEV] + [dict(
    id=DEV, name="Blue CHP kraftvärmeverk", host="127.0.0.1", port=1502, unitId=1,
    pollMs=1000, timeoutMs=2000, enabled=True, iec104=dict(port=2405, ca=2))]
cfg["tags"] = [t for t in cfg["tags"] if t["device"] != DEV] + tags
json.dump(cfg, open("data/config.json", "w", encoding="utf-8"), ensure_ascii=False, indent=2)

# ---------------------------------------------------------------- view
els = []
seq = [0]
FG, DIM, MUTED, PANEL = "#e6edf3", "#5c6b7a", "#8b949e", "#30363d"
HOT, COLD, GREEN, RED = "#ff6b6b", "#4dabf7", "#2ecc71", "#ff4d4d"
NODYN = dict(dynTag="", dynRules=[])
T = lambda id: PREFIX + id  # noqa: E731


def add(type, x, y, w, h, **props):
    seq[0] += 1
    els.append(dict(id=f"chp-{type}-{seq[0]}", type=type, x=x, y=y, w=w, h=h, rot=0, props=props))


def dyn(tag, on, off=DIM, op=">", value=0.5):
    other = {">": "<=", "==": "!="}[op]
    return dict(dynTag=T(tag), dynRules=[{"op": op, "value": value, "color": on}, {"op": other, "value": value, "color": off}])


def line(x, y, dx, dy, w=4, color=FG, d=None, dash=""):
    add("line", x, y, dx, dy, stroke=color, strokeWidth=w, dash=dash, **(d or NODYN))


def text(x, y, w, s, size=14, color=MUTED, bold=False, align="start", h=24):
    add("text", x, y, w, h, text=s, fontSize=size, color=color, bold=bold, align=align, **NODYN)


def panel(x, y, w, h, title):
    add("rect", x, y, w, h, fill="#00000000", stroke=PANEL, strokeWidth=1, radius=6, dash="", **NODYN)
    text(x + 12, y + 6, w - 24, title, 14, FG, True)


def m(x, y, tag, label, w=226, lo="", hi=""):
    add("measurement", x, y, w, 28, tag=T(tag), label=label, unit="", decimals="", fontSize=15, color=FG,
        background="#0d1117", border=PANEL, lowLimit=lo, highLimit=hi, alarmColor=RED)


def toggle(x, y, tag, label, on, off, w=226, color="#3d8bfd"):
    add("toggle", x, y, w, 28, label=label, statusTag=T(tag), openValue=0, closedValue=1, controlTag=T(tag + ".Cmd"),
        openCmd=0, closeCmd=1, offText=off, onText=on, colorClosed=color, colorOpen="#3a4450")


def ind(x, y, tag, label, on_color=RED, on_value=1, blink=True):
    add("indicator", x, y, 18, 18, tag=T(tag), onValue=on_value, onColor=on_color, offColor="#3a4450", label=label, blink=blink)


def button(x, y, w, label, tag, ask=False, color=PANEL):
    add("button", x, y, w, 28, text=label, controlTag=T(tag), value=1, askValue=ask, color=color, textColor=FG)


def setpoint(x, y, tag, label, w=226):
    m(x, y, tag, label, w - 68)
    button(x + w - 62, y, 62, "Ändra", tag, ask=True)


W, H = 1800, 1090
text(30, 18, 900, "Blue CHP – kraftvärmeverk", 26, FG, True, h=40)
add("link", 1620, 22, 150, 32, text="Blue Station ›", view="blue-station", color=PANEL, textColor=FG)

# ---- process graphics (top band) ----
HY = 95  # steam header
line(155, HY, 786, 0, 6, d=dyn("Steam.Pressure", FG, DIM, ">", 50))
text(560, HY - 30, 120, "Ångsamlare", 13)
m(700, HY - 52, "Steam.Pressure", "Tryck", 180, hi=97)
for i, (cx, name, fuel) in enumerate(((155, "P1", "Flis"), (417, "P2", "Bioolja"))):
    line(cx, HY, 0, 25, 5, d=dyn(f"{name}.SteamFlow", FG, DIM))
    add("rect", cx - 50, 120, 100, 150, fill="#161b22", stroke=FG, strokeWidth=2, radius=4, dash="", **dyn(f"{name}.Running", FG, DIM, "==", 1))
    text(cx - 40, 128, 80, name, 18, FG, True, "middle")
    text(cx - 40, 150, 80, fuel, 12, MUTED, False, "middle")
    add("bar", cx - 12, 178, 24, 80, tag=T(f"{name}.Load"), min=0, max=100, color="#ffa94d", background="#0d1117", showValue=False)
    ind(cx + 22, 182, f"{name}.Running", "", GREEN, blink=False)

# turbine, generator and connection to blue-station
TX = 941
line(TX, HY, 0, 25, 6, d=dyn("Turbine.SteamFlow", FG, DIM))
add("rect", TX - 45, 120, 90, 70, fill="#161b22", stroke=FG, strokeWidth=2, radius=4, dash="", **dyn("Turbine.AtSpeed", FG, DIM, "==", 1))
text(TX - 45, 143, 90, "Turbin", 14, FG, True, "middle")
line(TX + 45, 155, 187, 0, 4, color=MUTED)
add("ellipse", 1173, 125, 60, 60, fill="#161b22", stroke=FG, strokeWidth=3, **dyn("Gen.P", FG, DIM))
text(1173, 143, 60, "G", 20, FG, True, "middle")
line(1233, 155, 57, 0, 3, d=dyn("Gen.P", FG, DIM))
add("breaker", 1290, 135, 40, 40, label="Gen.CB", showLabel=False, statusTag=T("Gen.CB"), openValue=0, closedValue=1,
    controlTag=T("Gen.CB.Cmd"), openCmd=0, closeCmd=1, colorClosed=GREEN, colorOpen=GREEN)
line(1330, 155, 130, 0, 3, d=dyn("Net.Live", FG, DIM, "==", 1))
text(1470, 143, 300, "20 kV → Blue Station fack G1", 13, FG)

# condenser, bypass and district heating connections
add("rect", TX - 60, 225, 120, 46, fill="#161b22", stroke=FG, strokeWidth=2, radius=4, dash="", **NODYN)
text(TX - 60, 236, 120, "Kondensor", 13, FG, True, "middle")
line(TX, 190, 0, 35, 4, d=dyn("Turbine.SteamFlow", FG, DIM))
line(679, HY, 0, 153, 4, d=dyn("Bypass.Flow", FG, DIM), dash="8 5")
line(679, 248, 202, 0, 4, d=dyn("Bypass.Flow", FG, DIM), dash="8 5")
m(690, 150, "Bypass.Position", "Bypass", 150)
line(TX + 60, 248, 1720 - TX - 60, 0, 5, d=dyn("DH.Flow", HOT, DIM, ">", 10))  # supply out
line(1720, 248, 0, 572, 5, d=dyn("DH.Flow", HOT, DIM, ">", 10))
line(1760, 290, 0, 670, 5, d=dyn("DH.Flow", COLD, DIM, ">", 10))      # return in
line(TX, 290, 819, 0, 5, d=dyn("DH.Flow", COLD, DIM, ">", 10))
line(TX, 271, 0, 19, 5, d=dyn("DH.Flow", COLD, DIM, ">", 10))

# ---- panels (middle band) ----
PY, ROW = 310, 32


def rows(x, items):
    y = PY + 36
    for kind, *args in items:
        if kind == "m":
            m(x + 12, y, *args)
        elif kind == "sp":
            setpoint(x + 12, y, *args)
        elif kind == "t":
            toggle(x + 12, y, *args)
        elif kind == "i":
            ind(x + 14, y + 5, *args)
        elif kind == "b":
            button(x + 12, y, 226, *args)
        y += ROW


for i, (name, fuel, fault) in enumerate((("P1", "flis 120 MW", "Bränslematningsfel"), ("P2", "bioolja 50 MW", "Brännarfel"))):
    x = 30 + i * 262
    panel(x, PY, 250, 452, f"Panna {name} – {fuel}")
    rows(x, [
        ("t", f"{name}.Run", "Drift", "START", "STOPP", 226, GREEN),
        ("t", f"{name}.Auto", "Lastreglering", "AUTO", "MAN"),
        ("i", f"{name}.Running", "I drift", GREEN, 1, False),
        ("i", f"{name}.Trip", "Pannskydd utlöst"),
        ("m", f"{name}.Load", "Last"),
        ("m", f"{name}.FuelPower", "Bränsleeffekt"),
        ("m", f"{name}.FuelFlow", "Bränsleflöde"),
        ("m", f"{name}.SteamFlow", "Ångflöde"),
        ("m", f"{name}.SteamTemp", "Ångtemp"),
        ("m", f"{name}.O2", "O₂ rökgas"),
        ("m", f"{name}.FlueTemp", "Rökgastemp"),
        ("sp", f"{name}.LoadSet", "Last bv"),
        ("t", f"{name}.FaultSim", fault, "FEL", "AV", 226, "#c62828"),
    ])

x = 554
panel(x, PY, 250, 452, "Ångsamlare och bypass")
rows(x, [
    ("m", "Steam.Pressure", "Tryck"),
    ("m", "Steam.Flow", "Ångproduktion"),
    ("sp", "Steam.PressureSet", "Tryck bv"),
    ("i", "Steam.HighPressure", "Högt ångtryck"),
    ("t", "Bypass.Auto", "Bypass", "AUTO", "MAN"),
    ("m", "Bypass.Position", "Ventilläge"),
    ("m", "Bypass.Flow", "Ångflöde"),
    ("sp", "Bypass.ManualPos", "Läge man"),
])

x = 816
panel(x, PY, 250, 452, "Turbin")
rows(x, [
    ("t", "Turbine.Run", "Turbin", "START", "STOPP", 226, GREEN),
    ("m", "Turbine.Speed", "Varvtal"),
    ("m", "Turbine.SteamFlow", "Ångflöde"),
    ("i", "Turbine.AtSpeed", "På varvtal", GREEN, 1, False),
    ("i", "Turbine.Trip", "Turbinskydd utlöst"),
    ("b", "Snabbstopp", "Turbine.EmergencyStop", False, "#8b1e1e"),
])

x = 1078
panel(x, PY, 250, 452, "Generator")
rows(x, [
    ("m", "Gen.P", "P"),
    ("m", "Gen.Q", "Q"),
    ("m", "Gen.U", "U"),
    ("m", "Gen.f", "f"),
    ("m", "Gen.I", "I"),
    ("m", "Gen.Energy", "Energi"),
    ("sp", "Gen.QSet", "Q bv"),
    ("i", "Gen.Trip", "Nätbortfallsskydd utlöst"),
])
text(x + 12, PY + 36 + 8 * ROW + 4, 226, "Generatorbrytaren manövreras i bilden ovan", 11)

x = 1340
panel(x, PY, 330, 196, "Anslutning 20 kV")
rows(x, [
    ("i", "Net.Link", "Fristående (ingen länk)", "#ffb300", 0, False),
    ("i", "Net.Live", "Spänningslös anslutning", RED, 0),
    ("m", "Net.U", "U", 306),
    ("m", "Net.f", "f", 306),
])
panel(x, PY + 208, 330, 244, "Verk och simulering")
y = PY + 208 + 36
ind(x + 14, y + 5, "Plant.CommonAlarm", "Summalarm")
button(x + 176, y, 142, "Återställ", "Plant.Reset", color="#7a4b00")
setpoint(x + 12, y + ROW, "Env.Outdoor", "Utetemp", 306)
setpoint(x + 12, y + 2 * ROW, "Heat.Scale", "Värmebehov", 306)
setpoint(x + 12, y + 3 * ROW, "Acc.LevelSet", "Ack. nivå bv", 306)

# ---- district heating network (bottom band) ----
SY, RY = 820, 960  # supply / return pipes
line(1720, SY, -1440, 0, 5, d=dyn("DH.Flow", HOT, DIM, ">", 10))
line(280, RY, 1480, 0, 5, d=dyn("DH.Flow", COLD, DIM, ">", 10))
add("rect", 40, 790, 240, 200, fill="#161b22", stroke=FG, strokeWidth=2, radius=6, dash="", **NODYN)
text(52, 800, 216, "Fjärrvärmenät", 16, FG, True, "middle")
text(52, 824, 216, "kunder", 12, MUTED, False, "middle")
m(52, 860, "DH.Demand", "Behov", 216)
m(52, 894, "DH.Delivered", "Levererat", 216)
m(52, 928, "DH.DiffPressure", "Diff.tryck", 216, lo=0.6)
text(300, SY - 24, 200, "Framledning", 12, HOT)
text(300, RY + 6, 200, "Retur", 12, COLD)

col = [(300, [("DH.SupplyTemp", "Framledning"), ("DH.SupplyTempSet", "Framledning bv"), ("DH.ReturnTemp", "Retur")]),
       (540, [("DH.Production", "Produktion"), ("DH.Flow", "Flöde")])]
for cx, items in col:
    for j, (tg, lab) in enumerate(items):
        m(cx, 840 + j * 34, tg, lab, 226)
ind(790, 846, "DH.LowDP", "Lågt differenstryck")
ind(790, 880, "DH.SupplyLow", "Låg framledningstemp")
ind(790, 914, "DH.SupplyHigh", "Hög framledningstemp")

# pumps on the return pipe
for i, px in enumerate((1110, 1290)):
    n = i + 1
    add("ellipse", px - 15, RY - 15, 30, 30, fill="#161b22", stroke=COLD, strokeWidth=3, **dyn(f"DH.Pump{n}", GREEN, DIM, "==", 1))
    toggle(px - 85, RY + 24, f"DH.Pump{n}", f"Pump {n}", "DRIFT", "STOPP", 170, GREEN)

# heat accumulator between supply and return
AX = 1480
line(AX, SY, 0, 12, 4, color=HOT)
add("rect", AX - 35, SY + 12, 70, 116, fill="#161b22", stroke=FG, strokeWidth=2, radius=8, dash="", **NODYN)
add("bar", AX - 25, SY + 22, 50, 96, tag=T("Acc.Level"), min=0, max=100, color="#e8590c", background="#0d1117", showValue=False)
line(AX, SY + 128, 0, 12, 4, color=COLD)
text(AX - 70, RY + 24, 140, "Ackumulator", 12, MUTED, False, "middle")
m(AX + 48, SY + 14, "Acc.Level", "Nivå", 180)
m(AX + 48, SY + 48, "Acc.Energy", "Energi", 180)
m(AX + 48, SY + 82, "Acc.Power", "Effekt", 180)

text(30, H - 26, 1600, "Klicka på brytare, omkopplare och knappar för manöver, och på ett mätvärde för trend. "
     "Fylld brytare = Till, tom = Från. Lila = osäkert värde.", 12, DIM)

view = dict(id="blue-chp", name="Blue CHP", width=W, height=H, background="#0d1117", grid=10,
            elements=els, updated=datetime.datetime.now().astimezone().isoformat())
json.dump(view, open("data/views/blue-chp.json", "w", encoding="utf-8"), ensure_ascii=False, indent=2)
print(f"{len(tags)} taggar, {len(els)} element")
