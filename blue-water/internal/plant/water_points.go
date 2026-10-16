package plant

import "fmt"

func (s *Plant) pumpPoints(b *builder, p *Pump) {
	n := p.Name
	b.add(Point{ID: n + ".Run", Desc: p.Desc + " start/stopp", Kind: KindSwitch, OnText: "START", OffText: "STOPP",
		get: sp(func() bool { return p.Run }),
		ctrl: func(c Ctrl) error {
			if c.On && p.Trip {
				return ErrLockout
			}
			p.Run = c.On
			return nil
		}})
	b.add(Point{ID: n + ".Running", Desc: p.Desc + " i drift", Kind: KindStatus, OnText: "I DRIFT", OffText: "STOPPAD",
		get: sp(func() bool { return p.running })})
	b.add(Point{ID: n + ".Trip", Desc: p.Desc + " motorskydd utlöst", Kind: KindStatus, OnText: "UTLÖST", OffText: "NORMAL",
		get: sp(func() bool { return p.Trip })})
	b.add(Point{ID: n + ".Speed", Desc: p.Desc + " varvtal", Unit: "%", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return p.SpeedPct })})
	b.add(Point{ID: n + ".Flow", Desc: p.Desc + " flöde", Unit: "l/s", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return p.FlowLps })})
	b.add(Point{ID: n + ".Power", Desc: p.Desc + " effekt", Unit: "kW", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return p.PowerKW })})
	b.add(Point{ID: n + ".FaultSim", Desc: "Simulering: motorfel " + n, Kind: KindSwitch, OnText: "FEL", OffText: "INGET FEL",
		get: sp(func() bool { return p.FaultSim }), ctrl: setBool(&p.FaultSim)})
}

func (s *Plant) buildPoints() {
	cfg := s.cfg
	s.byID = map[string]*Point{}
	b := &builder{s: s, n: map[Kind]int{}}

	b.group = "Elmatning"
	b.add(Point{ID: "Supply.CB", Desc: "Inkommande brytare (elmatning)", Kind: KindBreaker, get: dp(&s.Supply),
		ctrl: func(c Ctrl) error { return s.Supply.operate(c.On) }})
	b.add(Point{ID: "Power.Available", Desc: "Simulering: extern elmatning", Kind: KindSwitch, OnText: "TILLGÄNGLIG", OffText: "BORTFALL",
		get: sp(func() bool { return s.PowerAvail }), ctrl: setBool(&s.PowerAvail)})
	b.add(Point{ID: "Power.Lost", Desc: "Verket spänningslöst", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL",
		get: sp(func() bool { return !s.powered() })})
	b.add(Point{ID: "Power.Total", Desc: "Total eleffekt", Unit: "kW", Kind: KindMeasurement, Deadband: 1, get: an(func() float64 { return s.PowerKW })})
	b.add(Point{ID: "Power.Energy", Desc: "Förbrukad elenergi", Unit: "MWh", Kind: KindMeasurement, Deadband: 0.001, get: an(func() float64 { return s.EnergyMWh })})

	b.group = "Råvattenintag"
	for _, p := range s.RP {
		s.pumpPoints(b, p)
	}
	b.add(Point{ID: "Raw.Auto", Desc: "Råvattenflöde styrs av reservoarnivå", Kind: KindSwitch, OnText: "AUTO", OffText: "MANUELL",
		get: sp(func() bool { return s.RawAuto }), ctrl: setBool(&s.RawAuto)})
	b.add(Point{ID: "Raw.Flow", Desc: "Råvattenflöde", Unit: "l/s", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return s.RawFlow })})
	b.add(Point{ID: "Raw.Turbidity", Desc: "Turbiditet råvatten", Unit: "NTU", Kind: KindMeasurement, Deadband: 0.05, get: an(func() float64 { return s.TurbRaw })})
	b.add(Point{ID: "Res.LevelSet", Desc: "Börvärde nivå rentvattenreservoar", Unit: "m", Kind: KindSetpoint,
		get: an(func() float64 { return s.LevelSetM }), ctrl: setFloat(&s.LevelSetM, 1, cfg.ReservoirM-0.5)})

	b.group = "Beredning (fällning och filtrering)"
	b.add(Point{ID: "Coag.Auto", Desc: "Fällningskemikalie dosering", Kind: KindSwitch, OnText: "AUTO", OffText: "MANUELL",
		get: sp(func() bool { return s.CoagAuto }), ctrl: setBool(&s.CoagAuto)})
	b.add(Point{ID: "Coag.Dose", Desc: "Dos fällningskemikalie", Unit: "mg/l", Kind: KindMeasurement, Deadband: 0.2, get: an(func() float64 { return s.CoagDose })})
	b.add(Point{ID: "Coag.DoseSet", Desc: "Dos fällningskemikalie vid manuell", Unit: "mg/l", Kind: KindSetpoint,
		get: an(func() float64 { return s.CoagSet }), ctrl: setFloat(&s.CoagSet, 0, 200)})
	b.add(Point{ID: "Settled.Turbidity", Desc: "Turbiditet efter sedimentering", Unit: "NTU", Kind: KindMeasurement, Deadband: 0.02,
		get: an(func() float64 { return s.TurbSettled })})
	for i := range s.F {
		f, other := s.F[i], s.F[1-i]
		n := f.Name
		b.add(Point{ID: n + ".Headloss", Desc: n + " filtermotstånd", Unit: "m", Kind: KindMeasurement, Deadband: 0.01, get: an(func() float64 { return f.HeadlossM })})
		b.add(Point{ID: n + ".Flow", Desc: n + " flöde", Unit: "l/s", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return f.FlowLps })})
		b.add(Point{ID: n + ".InService", Desc: n + " i drift", Kind: KindStatus, OnText: "I DRIFT", OffText: "BACKSPOLNING",
			get: sp(f.inService)})
		b.add(Point{ID: n + ".Clogged", Desc: fmt.Sprintf("%s igensatt (>%.1f m)", n, cfg.HeadlossAlarm), Kind: KindStatus, OnText: "LARM", OffText: "NORMAL",
			get: sp(func() bool { return s.clogged(f) })})
		b.add(Point{ID: n + ".Backwash", Desc: n + " starta backspolning", Kind: KindPulse,
			ctrl: func(c Ctrl) error {
				if !c.On {
					return nil
				}
				return s.backwash(f, other)
			}})
	}
	b.add(Point{ID: "Treated.Flow", Desc: "Flöde behandlat vatten", Unit: "l/s", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return s.TreatedFlow })})
	b.add(Point{ID: "Treated.Turbidity", Desc: "Turbiditet utgående vatten", Unit: "NTU", Kind: KindMeasurement, Deadband: 0.01, get: an(func() float64 { return s.TurbTreated })})
	b.add(Point{ID: "Treated.pH", Desc: "pH utgående vatten", Unit: "pH", Kind: KindMeasurement, Deadband: 0.02, get: an(func() float64 { return s.PH })})
	b.add(Point{ID: "Turb.High", Desc: "Hög turbiditet utgående vatten (>0,5 NTU)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.turbHigh)})

	b.group = "Desinfektion (klor)"
	b.add(Point{ID: "Cl.Auto", Desc: "Klordosering", Kind: KindSwitch, OnText: "AUTO", OffText: "MANUELL",
		get: sp(func() bool { return s.ClAuto }), ctrl: setBool(&s.ClAuto)})
	b.add(Point{ID: "Cl.Dose", Desc: "Klordos", Unit: "mg/l", Kind: KindMeasurement, Deadband: 0.01, get: an(func() float64 { return s.ClDose })})
	b.add(Point{ID: "Cl.DoseSet", Desc: "Klordos vid manuell", Unit: "mg/l", Kind: KindSetpoint,
		get: an(func() float64 { return s.ClSet }), ctrl: setFloat(&s.ClSet, 0, 5)})
	b.add(Point{ID: "Cl.ResidualSet", Desc: "Börvärde restklor", Unit: "mg/l", Kind: KindSetpoint,
		get: an(func() float64 { return s.ClTarget }), ctrl: setFloat(&s.ClTarget, 0.05, 1)})
	b.add(Point{ID: "Cl.Residual", Desc: "Restklor utgående vatten", Unit: "mg/l", Kind: KindMeasurement, Deadband: 0.01, get: an(func() float64 { return s.ClResidual })})
	b.add(Point{ID: "Cl.Low", Desc: "Låg restklor (<0,1 mg/l)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.clLow)})
	b.add(Point{ID: "Cl.High", Desc: "Hög restklor (>1,0 mg/l)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.clHigh)})

	b.group = fmt.Sprintf("Rentvattenreservoar %.0f m³", cfg.ReservoirM3)
	b.add(Point{ID: "Res.Level", Desc: "Nivå rentvattenreservoar", Unit: "m", Kind: KindMeasurement, Deadband: 0.01, get: an(s.level)})
	b.add(Point{ID: "Res.Volume", Desc: "Volym rentvattenreservoar", Unit: "m³", Kind: KindMeasurement, Deadband: 5, get: an(func() float64 { return s.ReservoirM3 })})
	b.add(Point{ID: "Res.Low", Desc: "Låg nivå reservoar (<1,0 m)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.resLow)})
	b.add(Point{ID: "Res.High", Desc: "Hög nivå reservoar (bräddning)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.resHigh)})

	b.group = "Distribution"
	for _, p := range s.HP {
		s.pumpPoints(b, p)
	}
	b.add(Point{ID: "Outlet.Valve", Desc: "Utgående ventil mot nätet", Kind: KindSwitch, OnText: "ÖPPEN", OffText: "STÄNGD",
		get: sp(func() bool { return s.OutletOpen }), ctrl: setBool(&s.OutletOpen)})
	b.add(Point{ID: "Net.Pressure", Desc: "Tryck distributionsnät", Unit: "bar", Kind: KindMeasurement, Deadband: 0.02, get: an(func() float64 { return s.PressBar })})
	b.add(Point{ID: "Net.PressureSet", Desc: "Börvärde nättryck", Unit: "bar", Kind: KindSetpoint,
		get: an(func() float64 { return s.PressSetBar }), ctrl: setFloat(&s.PressSetBar, 2, 7)})
	b.add(Point{ID: "Net.Flow", Desc: "Levererat flöde till nätet", Unit: "l/s", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return s.Delivered })})
	b.add(Point{ID: "Net.Demand", Desc: "Förbrukning i nätet", Unit: "l/s", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return s.Demand })})
	b.add(Point{ID: "Press.Low", Desc: "Lågt nättryck (<2,5 bar)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.pressLow)})

	b.group = "Simulering"
	b.add(Point{ID: "Raw.StormNTU", Desc: "Simulering: extra råvattenturbiditet (regn, storm)", Unit: "NTU", Kind: KindSetpoint,
		get: an(func() float64 { return s.StormNTU }), ctrl: setFloat(&s.StormNTU, 0, 100)})
	b.add(Point{ID: "Demand.Scale", Desc: "Simulering: skalning förbrukning", Unit: "%", Kind: KindSetpoint,
		get: an(func() float64 { return s.DemandScale }), ctrl: setFloat(&s.DemandScale, 0, 250)})

	b.group = "Verk"
	b.add(Point{ID: "Plant.CommonAlarm", Desc: "Summalarm", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.commonAlarm)})
	b.add(Point{ID: "Plant.Reset", Desc: "Återställ utlösningar", Kind: KindPulse,
		ctrl: func(c Ctrl) error {
			if c.On {
				s.reset()
			}
			return nil
		}})
}
