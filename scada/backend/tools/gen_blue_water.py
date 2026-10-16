"""Generates the blue-water device, tags and HMI view from the simulator's own
signal list. Run from scada/backend:

    python tools/gen_blue_water.py
"""
from scadagen import (DIM, DIRTY, FG, GREEN, MUTED, RED, WATER, View, build_tags, load_points, merge_config)

DEV, PREFIX = "blue-water", "WTR."

tags = build_tags(load_points(DEV), DEV, PREFIX)
merge_config(dict(id=DEV, name="Blue Water vattenverk", protocol="modbus", host="127.0.0.1", port=1503, unitId=1,
                  pollMs=1000, timeoutMs=2000, enabled=True, iec104=dict(port=2406, ca=3)), tags)

v = View("blue-water", "Blue Water", 1800, 1080, PREFIX, "wtr")
v.text(30, 18, 900, "Blue Water – vattenverk", 26, FG, True, h=40)
v.link(1470, 22, 140, "Blue WWTP ›", "blue-wwtp")
v.link(1620, 22, 150, "Blue Station ›", "blue-station")

flow = lambda tag: v.dyn(tag, WATER, DIM, ">", 1)  # noqa: E731
PY = 195  # main pipe level

# ---- process graphics ----
v.rect(30, 140, 120, 110, fill="#10283d", stroke=WATER)
v.text(30, 150, 120, "Sjö", 16, FG, True, "middle")
v.m(40, 205, "Raw.Turbidity", "Turb", 100)

v.line(150, PY, 25, 0, d=flow("Raw.Flow"))
v.line(175, 170, 0, 60, d=flow("Raw.Flow"))
v.line(255, 170, 0, 60, d=flow("Raw.Flow"))
for name, cy in (("RP1", 170), ("RP2", 230)):
    v.line(175, cy, 15, 0, d=flow(name + ".Flow"))
    v.pump(190, cy - 20, name)
    v.line(230, cy, 25, 0, d=flow(name + ".Flow"))
v.line(255, PY, 125, 0, d=flow("Raw.Flow"))
v.m(265, 110, "Coag.Dose", "Fällning", 150)
v.line(330, 140, 0, 50, 2, DIRTY, dash="4 3")

v.rect(380, 140, 170, 110, stroke=DIRTY)
v.text(380, 150, 170, "Sedimentering", 15, FG, True, "middle")
v.m(392, 205, "Settled.Turbidity", "Turb", 146)

v.line(550, PY, 50, 0, d=flow("Treated.Flow"))
v.line(600, 155, 0, 80, d=flow("Treated.Flow"))
v.line(735, 155, 0, 80, d=flow("Treated.Flow"))
for name, cy in (("F1", 155), ("F2", 235)):
    v.line(600, cy, 20, 0, d=flow(name + ".Flow"))
    v.rect(620, cy - 30, 95, 60, stroke=FG, d=v.dyn(name + ".InService", FG, RED, "==", 1))
    v.text(620, cy - 26, 95, name, 14, FG, True, "middle")
    v.bar(630, cy + 4, 75, 12, name + ".Headloss", 0, 3, "#e8590c")
    v.line(715, cy, 20, 0, d=flow(name + ".Flow"))
v.text(600, 272, 160, "Filtermotstånd 0–3 m", 11, MUTED)
v.line(735, PY, 165, 0, d=flow("Treated.Flow"))
v.m(755, 110, "Cl.Dose", "Klor", 130)
v.line(820, 140, 0, 50, 2, GREEN, dash="4 3")

v.rect(900, 110, 160, 170, stroke=WATER)
v.text(900, 116, 160, "Reservoar", 15, FG, True, "middle")
v.bar(960, 140, 40, 130, "Res.Level", 0, 6)

v.line(1060, PY, 30, 0, d=flow("Net.Flow"))
v.line(1090, 170, 0, 60, d=flow("Net.Flow"))
v.line(1170, 170, 0, 60, d=flow("Net.Flow"))
for name, cy in (("HP1", 170), ("HP2", 230)):
    v.line(1090, cy, 15, 0, d=flow(name + ".Flow"))
    v.pump(1105, cy - 20, name)
    v.line(1145, cy, 25, 0, d=flow(name + ".Flow"))
v.line(1170, PY, 30, 0, d=flow("Net.Flow"))
v.valve(1200, PY - 12, "Outlet.Valve", "Ventil")
v.line(1240, PY, 90, 0, d=flow("Net.Flow"))

v.rect(1330, 140, 160, 110, stroke=WATER)
v.text(1330, 148, 160, "Distributionsnät", 15, FG, True, "middle")
v.m(1340, 180, "Net.Pressure", "Tryck", 140, lo=2.5)
v.m(1340, 212, "Net.Demand", "Förbr.", 140)

v.text(1540, 112, 200, "Elmatning", 13, MUTED)
v.breaker(1560, 140, "Supply.CB", "Supply.CB")
v.ind(1540, 200, "Power.Lost", "Spänningslöst")

# ---- panels ----
PY0, ROW = 320, 32


v.form(30, PY0, "Råvattenintag", 250, [
    ("t", "Raw.Auto", "Flöde", "AUTO", "MAN"),
    ("m", "Raw.Flow", "Råvattenflöde"),
    ("sp", "Res.LevelSet", "Nivå bv"),
    ("m", "RP1.Speed", "RP1 varvtal"),
    ("m", "RP1.Power", "RP1 effekt"),
    ("i", "RP1.Trip", "RP1 motorskydd"),
    ("t", "RP1.FaultSim", "RP1 motorfel", "FEL", "AV", "#c62828"),
    ("m", "RP2.Speed", "RP2 varvtal"),
    ("m", "RP2.Power", "RP2 effekt"),
    ("i", "RP2.Trip", "RP2 motorskydd"),
    ("t", "RP2.FaultSim", "RP2 motorfel", "FEL", "AV", "#c62828"),
], h=420)

v.form(292, PY0, "Beredning", 250, [
    ("t", "Coag.Auto", "Fällning", "AUTO", "MAN"),
    ("sp", "Coag.DoseSet", "Dos bv"),
    ("m", "F1.Headloss", "F1 motstånd", "", 2.2),
    ("i", "F1.Clogged", "F1 igensatt"),
    ("b", "Backspola F1", "F1.Backwash"),
    ("m", "F2.Headloss", "F2 motstånd", "", 2.2),
    ("i", "F2.Clogged", "F2 igensatt"),
    ("b", "Backspola F2", "F2.Backwash"),
    ("m", "Treated.Turbidity", "Turb ut", "", 0.5),
    ("m", "Treated.pH", "pH"),
    ("i", "Turb.High", "Hög turbiditet"),
], h=420)

v.form(554, PY0, "Desinfektion", 250, [
    ("t", "Cl.Auto", "Klor", "AUTO", "MAN"),
    ("sp", "Cl.DoseSet", "Dos bv"),
    ("sp", "Cl.ResidualSet", "Restklor bv"),
    ("m", "Cl.Residual", "Restklor", 0.1, 1.0),
    ("i", "Cl.Low", "Låg restklor"),
    ("i", "Cl.High", "Hög restklor"),
], h=228)
v.panel(554, PY0 + 240, 250, 180, "Reservoar 8000 m³")
v.m(566, PY0 + 276, "Res.Level", "Nivå", 226, 1.0, 5.7)
v.m(566, PY0 + 308, "Res.Volume", "Volym", 226)
v.ind(568, PY0 + 345, "Res.Low", "Låg nivå")
v.ind(568, PY0 + 373, "Res.High", "Hög nivå (bräddning)")

v.form(816, PY0, "Distribution", 250, [
    ("sp", "Net.PressureSet", "Tryck bv"),
    ("m", "Net.Flow", "Levererat"),
    ("i", "Press.Low", "Lågt nättryck"),
    ("m", "HP1.Speed", "HP1 varvtal"),
    ("m", "HP1.Power", "HP1 effekt"),
    ("i", "HP1.Trip", "HP1 motorskydd"),
    ("t", "HP1.FaultSim", "HP1 motorfel", "FEL", "AV", "#c62828"),
    ("m", "HP2.Speed", "HP2 varvtal"),
    ("m", "HP2.Power", "HP2 effekt"),
    ("i", "HP2.Trip", "HP2 motorskydd"),
    ("t", "HP2.FaultSim", "HP2 motorfel", "FEL", "AV", "#c62828"),
], h=420)

v.form(1078, PY0, "Elmatning", 330, [
    ("m", "Power.Total", "Effekt"),
    ("m", "Power.Energy", "Energi"),
    ("t", "Power.Available", "Extern matning", "PÅ", "BORTA", GREEN),
], h=148)
v.panel(1078, PY0 + 160, 330, 260, "Verk och simulering")
y = PY0 + 196
v.ind(1092, y + 5, "Plant.CommonAlarm", "Summalarm")
v.button(1254, y, 142, "Återställ", "Plant.Reset", color="#7a4b00")
v.setpoint(1090, y + ROW, "Raw.StormNTU", "Storm (NTU)", 306)
v.setpoint(1090, y + 2 * ROW, "Demand.Scale", "Förbrukning", 306)

v.text(30, 1080 - 26, 1600, "Klicka på pumpar, ventil, brytare och knappar för manöver och på mätvärden för trend. "
       "Grön pump = i drift. Fylld brytare = Till. Lila = osäkert värde.", 12, DIM)

print(f"{len(tags)} taggar, {v.save()} element")
