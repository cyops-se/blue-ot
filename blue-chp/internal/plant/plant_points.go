package plant

import "fmt"

func (s *Plant) buildPoints() {
	cfg := s.cfg
	s.byID = map[string]*Point{}
	b := &builder{s: s, n: map[Kind]int{}}

	for i := range s.B {
		bo := s.B[i]
		n := bo.Name
		b.group = fmt.Sprintf("Panna %d – %s, %.0f MW", i+1, bo.Fuel, cfg.BoilerFuelMW[i])
		b.add(Point{ID: n + ".Run", Desc: n + " start/stopp", Kind: KindSwitch, OnText: "START", OffText: "STOPP",
			get: sp(func() bool { return bo.Run }),
			ctrl: func(c Ctrl) error {
				if c.On && bo.Trip {
					return ErrLockout
				}
				bo.Run = c.On
				return nil
			}})
		b.add(Point{ID: n + ".Auto", Desc: n + " lastreglering", Kind: KindSwitch, OnText: "AUTO", OffText: "MANUELL",
			get: sp(func() bool { return bo.Auto }), ctrl: setBool(&bo.Auto)})
		b.add(Point{ID: n + ".Running", Desc: n + " i drift", Kind: KindStatus, OnText: "I DRIFT", OffText: "EJ I DRIFT",
			get: sp(func() bool { return bo.State == BoilerRunning })})
		b.add(Point{ID: n + ".Trip", Desc: n + " pannskydd utlöst", Kind: KindStatus, OnText: "UTLÖST", OffText: "NORMAL",
			get: sp(func() bool { return bo.Trip })})
		b.add(Point{ID: n + ".State", Desc: n + " driftläge (0 stopp, 1 start, 2 drift, 3 stoppar)", Kind: KindMeasurement, Deadband: 0.5,
			get: an(func() float64 { return float64(bo.State) })})
		b.add(Point{ID: n + ".Load", Desc: n + " last", Unit: "%", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return bo.LoadPct })})
		b.add(Point{ID: n + ".FuelPower", Desc: n + " tillförd bränsleeffekt", Unit: "MW", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return bo.FuelMW })})
		b.add(Point{ID: n + ".FuelFlow", Desc: n + " bränsleflöde", Unit: "t/h", Kind: KindMeasurement, Deadband: 0.2,
			get: an(func() float64 { return bo.FuelMW / cfg.FuelLHV[bo.idx] })})
		b.add(Point{ID: n + ".SteamFlow", Desc: n + " ångflöde", Unit: "t/h", Kind: KindMeasurement, Deadband: 0.5,
			get: an(func() float64 { return bo.SteamMW / cfg.SteamMWhPerT })})
		b.add(Point{ID: n + ".SteamTemp", Desc: n + " färskångtemperatur", Unit: "°C", Kind: KindMeasurement, Deadband: 1, get: an(func() float64 { return bo.SteamC })})
		b.add(Point{ID: n + ".O2", Desc: n + " syrehalt rökgas", Unit: "%", Kind: KindMeasurement, Deadband: 0.1, get: an(func() float64 { return bo.O2 })})
		b.add(Point{ID: n + ".FlueTemp", Desc: n + " rökgastemperatur", Unit: "°C", Kind: KindMeasurement, Deadband: 1, get: an(func() float64 { return bo.FlueC })})
		b.add(Point{ID: n + ".LoadSet", Desc: n + " lastbörvärde (manuell)", Unit: "%", Kind: KindSetpoint,
			get: an(func() float64 { return bo.LoadSetPct }), ctrl: setFloat(&bo.LoadSetPct, cfg.BoilerMinPct, 100)})
		fault := "bränslematningsfel"
		if i == 1 {
			fault = "brännarfel"
		}
		b.add(Point{ID: n + ".FaultSim", Desc: "Simulering: " + fault + " " + n, Kind: KindSwitch, OnText: "FEL", OffText: "INGET FEL",
			get: sp(func() bool { return bo.FaultSim }), ctrl: setBool(&bo.FaultSim)})
	}

	b.group = "Ångsamlare och bypass"
	b.add(Point{ID: "Steam.Pressure", Desc: "Tryck ångsamlare", Unit: "bar", Kind: KindMeasurement, Deadband: 0.2, get: an(func() float64 { return s.PressBar })})
	b.add(Point{ID: "Steam.Flow", Desc: "Total ångproduktion", Unit: "t/h", Kind: KindMeasurement, Deadband: 0.5,
		get: an(func() float64 { return s.steamIn() / cfg.SteamMWhPerT })})
	b.add(Point{ID: "Steam.HighPressure", Desc: fmt.Sprintf("Högt ångtryck (>%.0f bar)", cfg.HeaderAlarmBar), Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.highPressure)})
	b.add(Point{ID: "Steam.PressureSet", Desc: "Börvärde ångtryck", Unit: "bar", Kind: KindSetpoint,
		get: an(func() float64 { return s.PressSetBar }), ctrl: setFloat(&s.PressSetBar, 70, 95)})
	b.add(Point{ID: "Bypass.Auto", Desc: "Bypassventil reglering", Kind: KindSwitch, OnText: "AUTO", OffText: "MANUELL",
		get: sp(func() bool { return s.BypassAuto }), ctrl: setBool(&s.BypassAuto)})
	b.add(Point{ID: "Bypass.Position", Desc: "Bypassventil läge", Unit: "%", Kind: KindMeasurement, Deadband: 0.5,
		get: an(func() float64 { return s.BypassMW / cfg.BypassMaxMW * 100 })})
	b.add(Point{ID: "Bypass.Flow", Desc: "Ångflöde bypass", Unit: "t/h", Kind: KindMeasurement, Deadband: 0.5,
		get: an(func() float64 { return s.BypassMW / cfg.SteamMWhPerT })})
	b.add(Point{ID: "Bypass.ManualPos", Desc: "Bypassventil läge vid manuell", Unit: "%", Kind: KindSetpoint,
		get: an(func() float64 { return s.BypassManPct }), ctrl: setFloat(&s.BypassManPct, 0, 100)})

	b.group = "Turbin och generator"
	b.add(Point{ID: "Turbine.Run", Desc: "Turbin start/stopp", Kind: KindSwitch, OnText: "START", OffText: "STOPP",
		get: sp(func() bool { return s.TurbineRun }),
		ctrl: func(c Ctrl) error {
			if c.On && s.TurbineTrip {
				return ErrLockout
			}
			if !c.On {
				s.openGen()
			}
			s.TurbineRun = c.On
			return nil
		}})
	b.add(Point{ID: "Turbine.Speed", Desc: "Turbinvarvtal", Unit: "rpm", Kind: KindMeasurement, Deadband: 5, get: an(func() float64 { return s.SpeedRPM })})
	b.add(Point{ID: "Turbine.SteamFlow", Desc: "Ångflöde turbin", Unit: "t/h", Kind: KindMeasurement, Deadband: 0.5,
		get: an(func() float64 { return s.TurbineMW / cfg.SteamMWhPerT })})
	b.add(Point{ID: "Turbine.AtSpeed", Desc: "Turbin på nominellt varvtal", Kind: KindStatus, OnText: "PÅ VARV", OffText: "EJ PÅ VARV",
		get: sp(func() bool { return s.SpeedRPM >= 0.98*cfg.RatedRPM })})
	b.add(Point{ID: "Turbine.Trip", Desc: "Turbinskydd utlöst", Kind: KindStatus, OnText: "UTLÖST", OffText: "NORMAL", get: sp(func() bool { return s.TurbineTrip })})
	b.add(Point{ID: "Turbine.EmergencyStop", Desc: "Turbin snabbstopp", Kind: KindPulse,
		ctrl: func(c Ctrl) error {
			if c.On && !s.TurbineTrip {
				s.tripTurbine("manuellt snabbstopp")
			}
			return nil
		}})
	b.add(Point{ID: "Gen.CB", Desc: "Generatorbrytare (synkronisering vid tillslag)", Kind: KindBreaker, get: dp(&s.GenCB), ctrl: func(c Ctrl) error { return s.closeGen(c.On) }})
	b.add(Point{ID: "Gen.P", Desc: "Generator aktiv effekt", Unit: "MW", Kind: KindMeasurement, Deadband: 0.1, get: an(func() float64 { return s.GenP })})
	b.add(Point{ID: "Gen.Q", Desc: "Generator reaktiv effekt", Unit: "Mvar", Kind: KindMeasurement, Deadband: 0.1, get: an(func() float64 { return s.GenQ })})
	b.add(Point{ID: "Gen.U", Desc: "Generatorspänning", Unit: "kV", Kind: KindMeasurement, Deadband: 0.05, get: an(func() float64 { return s.GenKV })})
	b.add(Point{ID: "Gen.f", Desc: "Generatorfrekvens", Unit: "Hz", Kind: KindMeasurement, Deadband: 0.01, get: an(func() float64 { return s.GenHz })})
	b.add(Point{ID: "Gen.I", Desc: "Generatorström", Unit: "A", Kind: KindMeasurement, Deadband: 2, get: an(func() float64 { return s.GenI })})
	b.add(Point{ID: "Gen.Energy", Desc: "Producerad elenergi", Unit: "MWh", Kind: KindMeasurement, Deadband: 0.01, get: an(func() float64 { return s.GenMWh })})
	b.add(Point{ID: "Gen.Trip", Desc: "Generatorskydd utlöst (nätbortfall)", Kind: KindStatus, OnText: "UTLÖST", OffText: "NORMAL", get: sp(func() bool { return s.GenTrip })})
	b.add(Point{ID: "Gen.QSet", Desc: "Börvärde reaktiv effekt", Unit: "Mvar", Kind: KindSetpoint,
		get: an(func() float64 { return s.GenQSet }), ctrl: setFloat(&s.GenQSet, -10, 15)})

	b.group = "Anslutning 20 kV (blue-station fack G1)"
	b.add(Point{ID: "Net.Link", Desc: "Länk till blue-station", Kind: KindStatus, OnText: "LÄNK", OffText: "FRISTÅENDE", get: sp(func() bool { return s.LinkOK })})
	b.add(Point{ID: "Net.Live", Desc: "Spänning på anslutningspunkten", Kind: KindStatus, OnText: "SPÄNNING", OffText: "SPÄNNINGSLÖS", get: sp(func() bool { return s.NetLive })})
	b.add(Point{ID: "Net.U", Desc: "Spänning anslutningspunkt", Unit: "kV", Kind: KindMeasurement, Deadband: 0.05, get: an(func() float64 { return s.NetKV })})
	b.add(Point{ID: "Net.f", Desc: "Frekvens anslutningspunkt", Unit: "Hz", Kind: KindMeasurement, Deadband: 0.01, get: an(func() float64 { return s.NetHz })})

	b.group = "Fjärrvärmenät"
	b.add(Point{ID: "DH.Demand", Desc: "Värmebehov nät (inkl. förluster)", Unit: "MW", Kind: KindMeasurement, Deadband: 0.2, get: an(func() float64 { return s.Demand })})
	b.add(Point{ID: "DH.Production", Desc: "Värmeproduktion (kondensor + bypass)", Unit: "MW", Kind: KindMeasurement, Deadband: 0.2, get: an(func() float64 { return s.Production })})
	b.add(Point{ID: "DH.Delivered", Desc: "Levererad värme till nätet", Unit: "MW", Kind: KindMeasurement, Deadband: 0.2, get: an(func() float64 { return s.Delivered })})
	b.add(Point{ID: "DH.SupplyTemp", Desc: "Framledningstemperatur", Unit: "°C", Kind: KindMeasurement, Deadband: 0.2, get: an(func() float64 { return s.SupplyC })})
	b.add(Point{ID: "DH.SupplyTempSet", Desc: "Börvärde framledning (utetemperaturkurva)", Unit: "°C", Kind: KindMeasurement, Deadband: 0.2, get: an(func() float64 { return s.SupplySetC })})
	b.add(Point{ID: "DH.ReturnTemp", Desc: "Returtemperatur", Unit: "°C", Kind: KindMeasurement, Deadband: 0.2, get: an(func() float64 { return s.ReturnC })})
	b.add(Point{ID: "DH.Flow", Desc: "Flöde fjärrvärmenät", Unit: "m³/h", Kind: KindMeasurement, Deadband: 5,
		get: an(func() float64 { return s.FlowKgS * 3.6 / 0.97 })})
	b.add(Point{ID: "DH.DiffPressure", Desc: "Differenstryck kritisk kund", Unit: "bar", Kind: KindMeasurement, Deadband: 0.02, get: an(func() float64 { return s.DPBar })})
	for i := range s.Pump {
		i := i
		b.add(Point{ID: fmt.Sprintf("DH.Pump%d", i+1), Desc: fmt.Sprintf("Fjärrvärmepump %d", i+1), Kind: KindSwitch, OnText: "DRIFT", OffText: "STOPP",
			get: sp(func() bool { return s.Pump[i] }), ctrl: setBool(&s.Pump[i])})
	}
	b.add(Point{ID: "DH.LowDP", Desc: "Lågt differenstryck i nätet (<0,6 bar)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.lowDP)})
	b.add(Point{ID: "DH.SupplyLow", Desc: "Låg framledningstemperatur (>8 °C under börvärde)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.supplyLow)})
	b.add(Point{ID: "DH.SupplyHigh", Desc: "Hög framledningstemperatur (>120 °C)", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.supplyHigh)})

	b.group = fmt.Sprintf("Värmeackumulator %.0f MWh", cfg.AccCapMWh)
	b.add(Point{ID: "Acc.Energy", Desc: "Lagrad energi", Unit: "MWh", Kind: KindMeasurement, Deadband: 0.5, get: an(func() float64 { return s.AccMWh })})
	b.add(Point{ID: "Acc.Level", Desc: "Laddningsgrad", Unit: "%", Kind: KindMeasurement, Deadband: 0.2, get: an(s.soc)})
	b.add(Point{ID: "Acc.Power", Desc: "Laddeffekt (+ laddning, − urladdning)", Unit: "MW", Kind: KindMeasurement, Deadband: 0.2, get: an(func() float64 { return s.AccMW })})
	b.add(Point{ID: "Acc.LevelSet", Desc: "Börvärde laddningsgrad", Unit: "%", Kind: KindSetpoint,
		get: an(func() float64 { return s.AccSOCSet }), ctrl: setFloat(&s.AccSOCSet, 10, 90)})

	b.group = "Simulering"
	b.add(Point{ID: "Env.Outdoor", Desc: "Simulering: utetemperatur", Unit: "°C", Kind: KindSetpoint,
		get: an(func() float64 { return s.OutdoorC }), ctrl: setFloat(&s.OutdoorC, -30, 25)})
	b.add(Point{ID: "Heat.Scale", Desc: "Simulering: skalning värmebehov", Unit: "%", Kind: KindSetpoint,
		get: an(func() float64 { return s.HeatScale }), ctrl: setFloat(&s.HeatScale, 0, 200)})

	b.group = "Verk"
	b.add(Point{ID: "Plant.CommonAlarm", Desc: "Summalarm", Kind: KindStatus, OnText: "LARM", OffText: "NORMAL", get: sp(s.commonAlarm)})
	b.add(Point{ID: "Plant.Reset", Desc: "Återställ utlösningar och blockeringar", Kind: KindPulse,
		ctrl: func(c Ctrl) error {
			if c.On {
				s.reset()
			}
			return nil
		}})
}
