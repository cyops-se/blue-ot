// Command modsim is a small Modbus TCP server that simulates a substation
// with four feeders, so the SCADA can be tested without real equipment.
//
// Register map (unit id is ignored):
//
//	Holding 100-103  breaker Q1-Q4 position (double point: 0=mellanläge, 1=öppen, 2=sluten, 3=fel)
//	Holding 200-203  breaker Q1-Q4 command (write 1=frånslag, 2=tillslag)
//	Discrete 0-3     disconnector QS1-QS4 position (true=sluten)
//	Coil 0-3         disconnector QS1-QS4 command (write true=slut, false=öppna)
//	Input 0..23      feeder 1-4 float32 (ABCD): U kV @ 6n, I A @ 6n+2, P MW @ 6n+4
//	Input 30         frequency Hz, float32
//	Input 32         busbar voltage kV, float32
//	Input 40         transformer temperature, int16 x10 (°C)
//
// A disconnector cannot be operated while its breaker is closed (interlock):
// the command is rejected with exception "illegal data value".
package main

import (
	"encoding/binary"
	"flag"
	"log"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/simonvetter/modbus"
)

const feeders = 4

type sim struct {
	mu       sync.Mutex
	breaker  [feeders]uint16 // DP position
	disc     [feeders]bool
	discCmd  [feeders]bool
	input    [64]uint16
	opDelay  time.Duration
	started  time.Time
	loadBase [feeders]float64
}

func newSim(delay time.Duration) *sim {
	s := &sim{opDelay: delay, started: time.Now(), loadBase: [feeders]float64{180, 120, 240, 90}}
	for i := range s.breaker {
		s.breaker[i] = 2
		s.disc[i] = true
		s.discCmd[i] = true
	}
	s.breaker[3] = 1 // Q4 open from start
	s.update()
	return s
}

func putFloat(regs []uint16, addr int, v float64) {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, math.Float32bits(float32(v)))
	regs[addr] = binary.BigEndian.Uint16(b[0:])
	regs[addr+1] = binary.BigEndian.Uint16(b[2:])
}

// update recalculates the measurements. Called with mu held.
func (s *sim) update() {
	t := time.Since(s.started).Seconds()
	busU := 10.5 + 0.08*math.Sin(t/20) + rand.NormFloat64()*0.01
	for i := 0; i < feeders; i++ {
		energized := s.breaker[i] == 2 && s.disc[i]
		u, cur := 0.0, 0.0
		if s.disc[i] {
			u = busU + rand.NormFloat64()*0.01
		}
		if energized {
			cur = s.loadBase[i]*(1+0.15*math.Sin(t/(15+float64(i)*7))) + rand.NormFloat64()*1.5
		}
		p := math.Sqrt(3) * u * cur * 0.95 / 1000
		putFloat(s.input[:], 6*i, u)
		putFloat(s.input[:], 6*i+2, cur)
		putFloat(s.input[:], 6*i+4, p)
	}
	putFloat(s.input[:], 30, 50+0.03*math.Sin(t/7)+rand.NormFloat64()*0.003)
	putFloat(s.input[:], 32, busU)
	s.input[40] = uint16(int16((55 + 8*math.Sin(t/60)) * 10))
}

func (s *sim) operateBreaker(i int, target uint16) {
	s.breaker[i] = 0 // travelling
	log.Printf("Q%d: manöver -> %d", i+1, target)
	time.AfterFunc(s.opDelay, func() {
		s.mu.Lock()
		s.breaker[i] = target
		s.update()
		s.mu.Unlock()
	})
}

func (s *sim) HandleCoils(req *modbus.CoilsRequest) ([]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if int(req.Addr)+int(req.Quantity) > feeders {
		return nil, modbus.ErrIllegalDataAddress
	}
	if req.IsWrite {
		for k, v := range req.Args {
			i := int(req.Addr) + k
			if s.breaker[i] != 1 && v != s.disc[i] {
				log.Printf("QS%d: förreglad (Q%d ej öppen)", i+1, i+1)
				return nil, modbus.ErrIllegalDataValue
			}
		}
		for k, v := range req.Args {
			i := int(req.Addr) + k
			s.discCmd[i] = v
			if v != s.disc[i] {
				log.Printf("QS%d: manöver -> %v", i+1, v)
				time.AfterFunc(s.opDelay*2, func() {
					s.mu.Lock()
					s.disc[i] = s.discCmd[i]
					s.update()
					s.mu.Unlock()
				})
			}
		}
		return nil, nil
	}
	return append([]bool(nil), s.discCmd[req.Addr:req.Addr+req.Quantity]...), nil
}

func (s *sim) HandleDiscreteInputs(req *modbus.DiscreteInputsRequest) ([]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if int(req.Addr)+int(req.Quantity) > feeders {
		return nil, modbus.ErrIllegalDataAddress
	}
	return append([]bool(nil), s.disc[req.Addr:req.Addr+req.Quantity]...), nil
}

func (s *sim) HandleHoldingRegisters(req *modbus.HoldingRegistersRequest) ([]uint16, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]uint16, req.Quantity)
	for k := 0; k < int(req.Quantity); k++ {
		a := int(req.Addr) + k
		switch {
		case a >= 100 && a < 100+feeders:
			if req.IsWrite {
				return nil, modbus.ErrIllegalDataAddress
			}
			res[k] = s.breaker[a-100]
		case a >= 200 && a < 200+feeders:
			if req.IsWrite {
				v := req.Args[k]
				if v != 1 && v != 2 {
					return nil, modbus.ErrIllegalDataValue
				}
				if s.breaker[a-200] != v {
					s.operateBreaker(a-200, v)
				}
			}
			// command registers always read back 0
		default:
			return nil, modbus.ErrIllegalDataAddress
		}
	}
	return res, nil
}

func (s *sim) HandleInputRegisters(req *modbus.InputRegistersRequest) ([]uint16, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if int(req.Addr)+int(req.Quantity) > len(s.input) {
		return nil, modbus.ErrIllegalDataAddress
	}
	return append([]uint16(nil), s.input[req.Addr:req.Addr+req.Quantity]...), nil
}

func main() {
	addr := flag.String("addr", "0.0.0.0:5020", "listen address")
	delay := flag.Duration("delay", 1500*time.Millisecond, "breaker operating time")
	flag.Parse()

	s := newSim(*delay)
	srv, err := modbus.NewServer(&modbus.ServerConfiguration{
		URL:        "tcp://" + *addr,
		Timeout:    60 * time.Second,
		MaxClients: 10,
	}, s)
	if err != nil {
		log.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}
	log.Printf("Modbus-simulator lyssnar på %s", *addr)

	go func() {
		for range time.Tick(500 * time.Millisecond) {
			s.mu.Lock()
			s.update()
			s.mu.Unlock()
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	_ = srv.Stop()
}
