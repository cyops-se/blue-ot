"""Generates the blue-station device, tags and HMI view from the signal list
in blue-station/README.md (repository root). Run from scada/backend:

    python tools/gen_blue_station.py
"""
import datetime
import json

DEV = "blue-station"

# ---------------------------------------------------------------- tags
tags = []


def tag(id, name, area, addr, dt="bool", unit="", dec=0, writable=False, log=False, state=None):
    t = dict(id=id, name=name, device=DEV, area=area, address=addr, dataType=dt, bit=-1,
             wordSwap=False, byteSwap=False, scale=1, offset=0, unit=unit, decimals=dec,
             writable=writable, logEvents=log)
    if state:
        t["stateText"] = state
    tags.append(t)


CB = {"0": "Från", "1": "Till"}
ONOFF = {"0": "Av", "1": "På"}
ALARM = {"0": "Normal", "1": "Larm"}
LIVE = {"0": "Spänningslös", "1": "Spänning"}
DEC = {"kV": 2, "Hz": 3, "MW": 2, "Mvar": 2, "A": 0, "%": 1, "°C": 1, "MWh": 1, "s": 0, "": 0}


def meas(id, name, ir, unit):
    tag(id, name, "input", ir, "float32", unit, DEC[unit])


def setpoint(id, name, hr, unit):
    tag(id, name, "holding", hr, "float32", unit, 2 if unit in ("kV", "%") else DEC[unit], writable=True)


def breaker(id, name, n):
    # status in discrete input n, command in coil n (1 = TILL)
    tag(id, name, "discrete", n, log=True, state=CB)
    tag(id + ".Cmd", name + " – manöver", "coil", n, writable=True)


def switch(id, name, n):
    tag(id, name, "discrete", n, log=True, state=ONOFF)
    tag(id + ".Cmd", name + " – manöver", "coil", n, writable=True)


def alarm(id, name, di):
    tag(id, name, "discrete", di, log=True, state=ALARM)


def pulse(id, name, coil):
    tag(id, name, "coil", coil, writable=True)


switch("Grid.Available", "Simulering: matning från stamnätet", 20)
tag("Grid.VoltagePresent", "Spänning på inkommande ledning", "discrete", 100, log=True, state=LIVE)
meas("Grid.U", "Spänning stamnät", 0, "kV")
meas("Grid.f", "Frekvens", 2, "Hz")
meas("Grid.P", "Aktiv effekt från stamnät", 4, "MW")
meas("Grid.Q", "Reaktiv effekt från stamnät", 6, "Mvar")
breaker("Q0.CB", "Linjebrytare Q0", 0)
meas("Q0.I", "Ström genom Q0", 8, "A")

TRAFOS = {
    # k: (hv coil, lv coil, first IR, tap IR, auto coil, raise coil, lower coil, first DI)
    1: (1, 2, 10, 200, 21, 40, 41, 101),
    2: (3, 4, 22, 202, 22, 42, 43, 104),
}
for k, (hv, lv, ir, tap_ir, auto, raise_, lower, di) in TRAFOS.items():
    T = f"T{k}"
    breaker(f"{T}.HV.CB", f"{T} brytare uppsida 220 kV", hv)
    breaker(f"{T}.LV.CB", f"{T} brytare nedsida 20 kV", lv)
    meas(f"{T}.Tap", f"{T} lindningskopplarläge", tap_ir, "")
    switch(f"{T}.AVR.Auto", f"{T} spänningsreglering auto", auto)
    pulse(f"{T}.Tap.Raise", f"{T} höj lindningskopplare", raise_)
    pulse(f"{T}.Tap.Lower", f"{T} sänk lindningskopplare", lower)
    meas(f"{T}.P", f"{T} aktiv effekt", ir, "MW")
    meas(f"{T}.Q", f"{T} reaktiv effekt", ir + 2, "Mvar")
    meas(f"{T}.I.HV", f"{T} ström uppsida", ir + 4, "A")
    meas(f"{T}.I.LV", f"{T} ström nedsida", ir + 6, "A")
    meas(f"{T}.Loading", f"{T} belastning", ir + 8, "%")
    meas(f"{T}.OilTemp", f"{T} oljetemperatur", ir + 10, "°C")
    alarm(f"{T}.Overload", f"{T} överlast", di)
    alarm(f"{T}.OilAlarm", f"{T} hög oljetemperatur", di + 1)
    alarm(f"{T}.Trip", f"{T} skyddsutlösning", di + 2)

breaker("BUS.Coupler.CB", "Sektioneringsbrytare A–B", 5)
tag("BUS.A.Live", "Spänning sektion A", "discrete", 107, log=True, state=LIVE)
meas("BUS.A.U", "Spänning sektion A", 34, "kV")
tag("BUS.B.Live", "Spänning sektion B", "discrete", 108, log=True, state=LIVE)
meas("BUS.B.U", "Spänning sektion B", 36, "kV")

FEEDERS = [("F1", "Bostad"), ("F2", "Industri"), ("F3", "Centrum"), ("F4", "Landsbygd")]
for i, (F, desc) in enumerate(FEEDERS):
    breaker(f"{F}.CB", f"{F} utgående brytare", 6 + i)
    ir = 38 + 8 * i
    meas(f"{F}.P", f"{F} aktiv effekt", ir, "MW")
    meas(f"{F}.Q", f"{F} reaktiv effekt", ir + 2, "Mvar")
    meas(f"{F}.I", f"{F} ström", ir + 4, "A")
    meas(f"{F}.Energy", f"{F} levererad energi", ir + 6, "MWh")
    alarm(f"{F}.Trip", f"{F} överströmsskydd utlöst", 109 + i)
    switch(f"{F}.FaultSim", f"Simulering: kortslutning på {F}", 23 + i)
    setpoint(f"{F}.OC.Pickup", f"{F} inställning I>", 2 * i, "A")

setpoint("AVR.USet", "Börvärde spänning 20 kV", 8, "kV")
setpoint("AVR.Deadband", "Dödband spänningsreglering", 10, "%")
setpoint("AVR.Delay", "Fördröjning spänningsreglering", 12, "s")
setpoint("Load.Scale", "Simulering: lastskalning", 14, "%")
setpoint("Env.Ambient", "Simulering: omgivningstemperatur", 16, "°C")
alarm("Station.CommonAlarm", "Summalarm", 113)
pulse("Station.Reset", "Återställ skyddsutlösningar", 44)

# merge into config.json (replacing earlier blue-station entries)
cfg = json.load(open("data/config.json", encoding="utf-8"))
cfg["devices"] = [d for d in cfg["devices"] if d["id"] != DEV] + [dict(
    id=DEV, name="Blue Station 220/20 kV", host="127.0.0.1", port=502, unitId=1,
    pollMs=1000, timeoutMs=2000, enabled=True)]
cfg["tags"] = [t for t in cfg["tags"] if t["device"] != DEV] + tags
json.dump(cfg, open("data/config.json", "w", encoding="utf-8"), ensure_ascii=False, indent=2)

# ---------------------------------------------------------------- view
els = []
seq = [0]
FG, DIM, MUTED, PANEL = "#e6edf3", "#5c6b7a", "#8b949e", "#30363d"
NODYN = dict(dynTag="", dynRules=[])


def add(type, x, y, w, h, **props):
    seq[0] += 1
    els.append(dict(id=f"bs-{type}-{seq[0]}", type=type, x=x, y=y, w=w, h=h, rot=0, props=props))


def live(tag, op=">", value=0.5):
    """Dynamic color: energized = white, otherwise grey."""
    other = {">": "<=", "==": "!="}[op]
    return dict(dynTag=tag, dynRules=[{"op": op, "value": value, "color": FG},
                                      {"op": other, "value": value, "color": DIM}])


def line(x, y, dx, dy, w=3, dyn=None, color=FG):
    add("line", x, y, dx, dy, stroke=color, strokeWidth=w, dash="", **(dyn or NODYN))


def text(x, y, w, s, size=14, color=MUTED, bold=False, align="start", h=24):
    add("text", x, y, w, h, text=s, fontSize=size, color=color, bold=bold, align=align, **NODYN)


def panel(x, y, w, h, title):
    add("rect", x, y, w, h, fill="#00000000", stroke=PANEL, strokeWidth=1, radius=6, dash="", **NODYN)
    text(x + 12, y + 6, w - 24, title, 14, FG, True)


def m(x, y, tag, label, w=200, h=28, lo="", hi=""):
    add("measurement", x, y, w, h, tag=tag, label=label, unit="", decimals="", fontSize=15, color=FG,
        background="#0d1117", border=PANEL, lowLimit=lo, highLimit=hi, alarmColor="#ff4d4d")


def cb(x, y, status, label, show=True):
    add("breaker", x, y, 40, 40, label=label, showLabel=show, statusTag=status, openValue=0, closedValue=1,
        controlTag=status + ".Cmd", openCmd=0, closeCmd=1, colorClosed="#ff4d4d", colorOpen="#2ecc71")


def toggle(x, y, status, label, w=200, on="PÅ", off="AV", color="#3d8bfd"):
    add("toggle", x, y, w, 28, label=label, statusTag=status, openValue=0, closedValue=1,
        controlTag=status + ".Cmd", openCmd=0, closeCmd=1, offText=off, onText=on,
        colorClosed=color, colorOpen="#3a4450")


def alarm_ind(x, y, tag, label):
    add("indicator", x, y, 18, 18, tag=tag, onValue=1, onColor="#ff4d4d", offColor="#3a4450", label=label, blink=True)


def button(x, y, w, label, tag, ask=False, value=1, color=PANEL):
    add("button", x, y, w, 28, text=label, controlTag=tag, value=value, askValue=ask, color=color, textColor=FG)


W, H = 1800, 1060
text(30, 18, 900, "Blue Station – 220/20 kV transformatorstation", 26, FG, True, h=40)
add("link", 1620, 22, 150, 32, text="Station Norra", view="station", color=PANEL, textColor=FG)

# station / AVR panel (top left)
panel(30, 70, 470, 200, "Station och spänningsreglering")
alarm_ind(50, 108, "Station.CommonAlarm", "Summalarm")
button(330, 104, 150, "Återställ", "Station.Reset", color="#7a4b00")
m(50, 145, "AVR.USet", "U börvärde", 260)
button(320, 145, 80, "Ändra", "AVR.USet", ask=True)
m(50, 180, "AVR.Deadband", "Dödband", 260)
button(320, 180, 80, "Ändra", "AVR.Deadband", ask=True)
m(50, 215, "AVR.Delay", "Fördröjning", 260)
button(320, 215, 80, "Ändra", "AVR.Delay", ask=True)

# simulation panel (top right)
panel(1300, 70, 470, 200, "Simulering")
toggle(1320, 108, "Grid.Available", "Matning stamnät", 260, on="PÅ", off="BORTA", color="#2e7d32")
m(1320, 145, "Load.Scale", "Lastskalning", 260)
button(1590, 145, 80, "Ändra", "Load.Scale", ask=True)
m(1320, 180, "Env.Ambient", "Omgivningstemp", 260)
button(1590, 180, 80, "Ändra", "Env.Ambient", ask=True)

# grid infeed and Q0
GX = 900
text(GX + 14, 66, 200, "Stamnät 220 kV", 15, FG, True)
line(GX, 60, 0, 120, dyn=live("Grid.VoltagePresent"))
cb(GX - 20, 180, "Q0.CB", "Q0")
line(GX, 220, 0, 60, dyn=live("Q0.CB", "==", 1))
m(960, 100, "Grid.U", "U", 170)
m(960, 132, "Grid.f", "f", 170, lo=49.8, hi=50.2)
m(1140, 100, "Grid.P", "P", 140)
m(1140, 132, "Grid.Q", "Q", 140)
m(960, 164, "Q0.I", "I Q0", 170)

# 220 kV busbar
BUS_HV = 280
text(400, BUS_HV - 12, 60, "220 kV", 13)
add("busbar", 460, BUS_HV, 880, 0, stroke=FG, strokeWidth=8, dash="", **live("Q0.CB", "==", 1))

# transformers with info panels
BUS_LV = 640
for k, TX, PX in ((1, 600, 30), (2, 1200, 1300)):
    T = f"T{k}"
    sec = "A" if k == 1 else "B"
    line(TX, BUS_HV, 0, 50, dyn=live("Q0.CB", "==", 1))
    cb(TX - 20, 330, f"{T}.HV.CB", "", show=False)
    line(TX, 370, 0, 30, dyn=live(f"{T}.I.HV", ">", 0.5))
    add("transformer", TX - 30, 400, 60, 100, stroke=FG, strokeWidth=3, **live(f"{T}.I.HV", ">", 0.5))
    text(TX + 38, 438, 80, T, 18, FG, True)
    line(TX, 500, 0, 30, dyn=live(f"{T}.I.LV", ">", 0.5))
    cb(TX - 20, 530, f"{T}.LV.CB", "", show=False)
    line(TX, 570, 0, BUS_LV - 570, dyn=live(f"BUS.{sec}.Live"))
    text(TX + 26, 340, 60, "HV", 12)
    text(TX + 26, 540, 60, "LV", 12)

    panel(PX, 300, 470, 300, f"{T}  220/20 kV  40 MVA")
    x1, x2 = PX + 20, PX + 250
    m(x1, 340, f"{T}.P", "P")
    m(x1, 372, f"{T}.Q", "Q")
    m(x1, 404, f"{T}.I.HV", "I uppsida")
    m(x1, 436, f"{T}.I.LV", "I nedsida")
    m(x1, 468, f"{T}.Loading", "Belastning", hi=100)
    m(x1, 500, f"{T}.OilTemp", "Oljetemp", hi=90)
    add("bar", x1, 540, 200, 14, tag=f"{T}.Loading", min=0, max=150, color="#3d8bfd", background="#0d1117", showValue=False)
    text(x1, 558, 200, "Belastning 0–150 %", 11)
    m(x2, 340, f"{T}.Tap", "Lindn.kopplare", 200)
    button(x2, 372, 96, "▲ Höj", f"{T}.Tap.Raise")
    button(x2 + 104, 372, 96, "▼ Sänk", f"{T}.Tap.Lower")
    toggle(x2, 408, f"{T}.AVR.Auto", "AVR", 200, on="AUTO", off="MAN")
    alarm_ind(x2, 452, f"{T}.Overload", "Överlast")
    alarm_ind(x2, 480, f"{T}.OilAlarm", "Hög oljetemp")
    alarm_ind(x2, 508, f"{T}.Trip", "Skyddsutlösning")

# 20 kV busbars and bus coupler
text(40, BUS_LV - 30, 150, "20 kV sektion A", 13)
m(40, BUS_LV + 14, "BUS.A.U", "U A", 150)
add("busbar", 200, BUS_LV, 660, 0, stroke=FG, strokeWidth=8, dash="", **live("BUS.A.Live"))
line(860, BUS_LV, 20, 0, 4, dyn=live("BUS.A.Live"))
cb(880, BUS_LV - 20, "BUS.Coupler.CB", "", show=False)
line(920, BUS_LV, 20, 0, 4, dyn=live("BUS.B.Live"))
text(840, BUS_LV + 26, 120, "Sektionering", 12, align="middle")
add("busbar", 940, BUS_LV, 660, 0, stroke=FG, strokeWidth=8, dash="", **live("BUS.B.Live"))
text(1620, BUS_LV - 30, 150, "20 kV sektion B", 13)
m(1620, BUS_LV + 14, "BUS.B.U", "U B", 150)

# feeders
for (F, desc), FX, bus in zip(FEEDERS, (330, 660, 1100, 1430), "AABB"):
    line(FX, BUS_LV, 0, 40, dyn=live(f"BUS.{bus}.Live"))
    cb(FX - 20, BUS_LV + 40, f"{F}.CB", F)
    line(FX, BUS_LV + 80, 0, 50, dyn=live(f"{F}.I", ">", 0.5))
    # outgoing arrow
    add("line", FX - 6, BUS_LV + 122, 6, 8, stroke=MUTED, strokeWidth=3, dash="", **NODYN)
    add("line", FX + 6, BUS_LV + 122, -6, 8, stroke=MUTED, strokeWidth=3, dash="", **NODYN)
    text(FX - 100, BUS_LV + 136, 200, f"{F} {desc}", 17, FG, True, "middle", 26)
    y = BUS_LV + 168
    m(FX - 100, y, f"{F}.P", "P")
    m(FX - 100, y + 32, f"{F}.Q", "Q")
    m(FX - 100, y + 64, f"{F}.I", "I")
    m(FX - 100, y + 96, f"{F}.Energy", "Energi")
    alarm_ind(FX - 98, y + 134, f"{F}.Trip", "Överström utlöst")
    m(FX - 100, y + 162, f"{F}.OC.Pickup", "I>", 130)
    button(FX + 36, y + 162, 64, "Ändra", f"{F}.OC.Pickup", ask=True)
    toggle(FX - 100, y + 196, f"{F}.FaultSim", "Felsimulering", 200, on="FEL", off="AV", color="#c62828")

text(30, H - 26, 1400, "Klicka på brytare, omkopplare och knappar för manöver, och på ett mätvärde för trend. "
     "Röd brytare = Till, grön = Från. Lila = osäkert värde.", 12, DIM)

view = dict(id="blue-station", name="Blue Station", width=W, height=H, background="#0d1117", grid=10,
            elements=els, updated=datetime.datetime.now().astimezone().isoformat())
json.dump(view, open("data/views/blue-station.json", "w", encoding="utf-8"), ensure_ascii=False, indent=2)
print(f"{len(tags)} taggar, {len(els)} element")
