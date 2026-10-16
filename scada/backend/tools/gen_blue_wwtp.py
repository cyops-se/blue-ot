"""Generates the blue-wwtp device, tags and HMI view from the simulator's own
signal list. Run from scada/backend:

    python tools/gen_blue_wwtp.py
"""
from scadagen import (AIR, DIM, DIRTY, FG, GAS, GREEN, MUTED, RED, WATER, View, build_tags, load_points, merge_config)

DEV, PREFIX = "blue-wwtp", "WWT."

tags = build_tags(load_points(DEV), DEV, PREFIX)
merge_config(dict(id=DEV, name="Blue WWTP avloppsverk", protocol="modbus", host="127.0.0.1", port=1504, unitId=1,
                  pollMs=1000, timeoutMs=2000, enabled=True, iec104=dict(port=2407, ca=4)), tags)

v = View("blue-wwtp", "Blue WWTP", 1800, 1080, PREFIX, "wwt")
v.text(30, 18, 900, "Blue WWTP – avloppsreningsverk", 26, FG, True, h=40)
v.link(1470, 22, 140, "Blue Water ›", "blue-water")
v.link(1620, 22, 150, "Blue Station ›", "blue-station")

dirty = lambda tag: v.dyn(tag, DIRTY, DIM, ">", 1)  # noqa: E731
clean = lambda tag: v.dyn(tag, WATER, DIM, ">", 1)  # noqa: E731
PY = 195  # main channel level

# ---- inlet pump station ----
v.text(30, 112, 120, "Inkommande", 13, MUTED)
v.m(30, 140, "Inflow.Flow", "Flöde", 120)
v.line(150, PY, 30, 0, d=dirty("Inflow.Flow"))
v.rect(180, 120, 70, 150, stroke=DIRTY)
v.text(180, 124, 70, "Brunn", 13, FG, True, "middle")
v.bar(200, 150, 30, 110, "Well.Level", 0, 4, DIRTY)

v.line(250, PY, 20, 0, d=dirty("Pumped.Flow"))
v.line(270, 135, 0, 120, d=dirty("Pumped.Flow"))
v.line(350, 135, 0, 120, d=dirty("Pumped.Flow"))
for name, cy in (("IP1", 135), ("IP2", 195), ("IP3", 255)):
    v.line(270, cy, 15, 0, d=dirty(name + ".Flow"))
    v.pump(285, cy - 20, name, size=36)
    v.line(321, cy, 29, 0, d=dirty(name + ".Flow"))

# overflow from the well to the recipient
v.line(215, 270, 0, 70, 3, RED, v.dyn("Overflow.Active", RED, DIM, "==", 1), "6 4")
v.text(225, 280, 160, "Bräddning", 12, MUTED)
v.m(225, 304, "Overflow.Flow", "Bräddflöde", 170, hi=0.5)

# ---- screen ----
v.line(350, PY, 40, 0, d=dirty("Pumped.Flow"))
v.rect(390, 150, 90, 90, stroke=DIRTY, d=v.dyn("Screen.Blocked", RED, DIRTY, "==", 1))
v.text(390, 156, 90, "Galler", 14, FG, True, "middle")
v.ind(402, 200, "Screen.Running", "Drift", GREEN, blink=False)

# ---- biology with blowers ----
v.line(480, PY, 60, 0, d=dirty("Bio.Flow"))
v.rect(540, 130, 260, 130, fill="#1b2416", stroke="#8bc34a")
v.text(540, 136, 260, "Biologisk rening (luftning)", 14, FG, True, "middle")
v.m(556, 166, "Bio.DO", "Syre", 228, 1.0)
v.m(556, 200, "Air.Flow", "Luft", 228)
v.line(668, 260, 0, 40, 3, AIR, v.dyn("Air.Flow", AIR, DIM, ">", 10))
v.line(618, 300, 100, 0, 3, AIR, v.dyn("Air.Flow", AIR, DIM, ">", 10))
for name, x in (("B1", 600), ("B2", 700)):
    v.line(x + 18, 300, 0, 10, 3, AIR, v.dyn(name + ".Air", AIR, DIM, ">", 10))
    v.pump(x, 310, name, size=36)

# ---- precipitation and settling ----
v.line(800, PY, 120, 0, d=dirty("Bio.Flow"))
v.m(810, 110, "Prec.Dose", "Fällning", 150)
v.line(870, 140, 0, 50, 2, GAS, dash="4 3")
v.rect(920, 140, 160, 110, stroke=WATER)
v.text(920, 146, 160, "Eftersedimentering", 14, FG, True, "middle")
v.m(932, 190, "Eff.BOD7", "BOD7", 136)

# ---- effluent ----
v.line(1080, PY, 120, 0, d=clean("Eff.Flow"))
v.rect(1200, 120, 170, 150, fill="#10283d", stroke=WATER)
v.text(1200, 126, 170, "Recipient", 15, FG, True, "middle")
v.m(1210, 160, "Eff.NH4", "NH4-N", 150, hi=5)
v.m(1210, 194, "Eff.Ptot", "Ptot", 150, hi=0.3)
v.m(1210, 228, "Eff.Flow", "Flöde", 150)

# ---- sludge and digestion ----
v.line(1000, 250, 0, 160, 3, DIRTY, dash="6 4")
v.line(1000, 410, 60, 0, 3, DIRTY, dash="6 4")
v.text(1006, 380, 120, "Slam", 12, MUTED)
v.rect(1060, 360, 150, 110, fill="#241a10", stroke=DIRTY)
v.text(1060, 366, 150, "Rötkammare", 14, FG, True, "middle")
v.m(1070, 396, "Digester.Temp", "Temp", 130, 34)
v.ind(1072, 436, "Boiler.Run", "Panna", GREEN, blink=False)
v.line(1210, 410, 60, 0, 3, GAS, v.dyn("Biogas.Flow", GAS, DIM, ">", 1))
v.rect(1270, 350, 90, 120, stroke=GAS)
v.text(1270, 356, 90, "Gasklocka", 13, FG, True, "middle")
v.bar(1300, 380, 30, 80, "Gasholder.Level", 0, 100, GAS)
v.line(1360, 410, 50, 0, 3, GAS, v.dyn("Gas.Flaring", RED, DIM, "==", 1))
v.ind(1412, 401, "Gas.Flaring", "Fackling", "#ff922b")

# ---- power supply ----
v.text(1540, 112, 200, "Elmatning", 13, MUTED)
v.breaker(1560, 140, "Supply.CB", "Supply.CB")
v.ind(1540, 200, "Power.Lost", "Spänningslöst")

# ---- panels ----
PY0 = 500
FAULT = "#c62828"
v.form(30, PY0, "Inloppspumpstation", 260, [
    ("t", "Inlet.Auto", "Pumpar", "AUTO", "MAN"),
    ("m", "Well.Level", "Nivå brunn", "", 3.5),
    ("i", "Well.High", "Hög nivå brunn"),
    ("m", "Pumped.Flow", "Pumpat"),
    ("m", "Overflow.Volume", "Bräddat"),
    ("i", "Overflow.Active", "Bräddning pågår"),
    ("m", "IP1.Power", "IP1 effekt"),
    ("i", "IP1.Trip", "IP1 motorskydd"),
    ("t", "IP1.FaultSim", "IP1 motorfel", "FEL", "AV", FAULT),
    ("m", "IP2.Power", "IP2 effekt"),
    ("i", "IP2.Trip", "IP2 motorskydd"),
    ("t", "IP2.FaultSim", "IP2 motorfel", "FEL", "AV", FAULT),
    ("m", "IP3.Power", "IP3 effekt"),
    ("i", "IP3.Trip", "IP3 motorskydd"),
    ("t", "IP3.FaultSim", "IP3 motorfel", "FEL", "AV", FAULT),
], h=532)

v.form(302, PY0, "Biologi och galler", 260, [
    ("t", "Blower.Auto", "Blåsmaskiner", "AUTO", "MAN"),
    ("sp", "Bio.DOSet", "Syre bv"),
    ("m", "Bio.DO", "Syrehalt", 1.0),
    ("i", "Bio.DOLow", "Låg syrehalt"),
    ("m", "B1.Air", "B1 luft"),
    ("m", "B1.Power", "B1 effekt"),
    ("i", "B1.Trip", "B1 motorskydd"),
    ("t", "B1.FaultSim", "B1 motorfel", "FEL", "AV", FAULT),
    ("m", "B2.Air", "B2 luft"),
    ("m", "B2.Power", "B2 effekt"),
    ("i", "B2.Trip", "B2 motorskydd"),
    ("t", "B2.FaultSim", "B2 motorfel", "FEL", "AV", FAULT),
    ("i", "Screen.Blocked", "Galler igensatt"),
    ("t", "Screen.FaultSim", "Gallerfel", "FEL", "AV", FAULT),
], h=500)

v.form(574, PY0, "Kemisk fällning", 260, [
    ("t", "Prec.Auto", "Dosering", "AUTO", "MAN"),
    ("sp", "Prec.DoseSet", "Dos bv"),
    ("m", "Prec.Dose", "Dos"),
], h=148)
v.form(574, PY0 + 160, "Utgående vatten", 260, [
    ("m", "In.NH4", "NH4-N in"),
    ("m", "In.Ptot", "Ptot in"),
    ("m", "Eff.NH4", "NH4-N ut", "", 5),
    ("m", "Eff.Ptot", "Ptot ut", "", 0.3),
    ("i", "Eff.NH4High", "NH4-N över gräns"),
    ("i", "Eff.PtotHigh", "Ptot över gräns"),
], h=340)

v.form(846, PY0 + 0, "Rötning", 260, [
    ("t", "Boiler.Run", "Panna", "DRIFT", "STOPP", GREEN),
    ("m", "Digester.Temp", "Temp", 34),
    ("i", "Digester.TempLow", "Låg rötkammartemp"),
    ("m", "Biogas.Flow", "Biogas"),
    ("m", "Gasholder.Level", "Gasklocka", "", 95),
    ("i", "Gas.Flaring", "Fackling"),
], h=240)

v.form(1118, PY0, "Elmatning", 300, [
    ("m", "Power.Total", "Effekt"),
    ("m", "Power.Energy", "Energi"),
    ("t", "Power.Available", "Extern matning", "PÅ", "BORTA", GREEN),
], h=148)
v.panel(1118, PY0 + 160, 300, 180, "Verk och simulering")
y = PY0 + 196
v.ind(1132, y + 5, "Plant.CommonAlarm", "Summalarm")
v.button(1264, y, 142, "Återställ", "Plant.Reset", color="#7a4b00")
v.setpoint(1130, y + 32, "Rain.Intensity", "Regn", 276)
v.setpoint(1130, y + 64, "Load.Scale", "Belastning", 276)
v.setpoint(1130, y + 96, "Water.Temp", "Vattentemp", 276)

v.text(30, 1080 - 26, 1600, "Klicka på pumpar, blåsmaskiner, brytare och knappar för manöver och på mätvärden för trend. "
       "Grön = i drift. Röd streckad linje = bräddning. Lila = osäkert värde.", 12, DIM)

print(f"{len(tags)} taggar, {v.save()} element")
