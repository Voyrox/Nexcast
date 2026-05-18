package nextcast

import (
	"sync"
	"time"
)

type HoltWintersState struct {
	Level  float64
	Trend  float64
	Hourly [24]float64
	Weekly [7]float64
	RPSMax float64
}

type HoltWintersPredictor struct {
	mu      sync.RWMutex
	states  map[string]*HoltWintersState
	Alpha   float64
	Beta    float64
	Gamma   float64
	Delta   float64
	Horizon int
}

func NewHoltWintersPredictor(alpha, beta, gamma, delta float64, horizon int) *HoltWintersPredictor {
	return &HoltWintersPredictor{
		states:  make(map[string]*HoltWintersState),
		Alpha:   alpha,
		Beta:    beta,
		Gamma:   gamma,
		Delta:   delta,
		Horizon: horizon,
	}
}

func (p *HoltWintersPredictor) Update(serviceName string, rps float64, ts time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	s, ok := p.states[serviceName]
	if !ok {
		s = &HoltWintersState{
			Level:  rps,
			Trend:  0,
			Hourly: [24]float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			Weekly: [7]float64{1, 1, 1, 1, 1, 1, 1},
			RPSMax: rps,
		}
		p.states[serviceName] = s
		return
	}

	h := ts.Hour()
	w := int(ts.Weekday())

	seasonal := s.Hourly[h] * s.Weekly[w]
	if seasonal < 0.01 {
		seasonal = 0.01
	}
	deseasonalized := rps / seasonal

	prevLevel := s.Level
	s.Level = p.Alpha*deseasonalized + (1-p.Alpha)*(s.Level+s.Trend)
	s.Trend = p.Beta*(s.Level-prevLevel) + (1-p.Beta)*s.Trend

	if s.Level > 0 {
		implied := rps / s.Level

		s.Hourly[h] = p.Gamma*implied + (1-p.Gamma)*s.Hourly[h]
		if s.Hourly[h] < 0.01 {
			s.Hourly[h] = 0.01
		}

		hAdj := s.Hourly[h]
		if hAdj < 0.01 {
			hAdj = 0.01
		}
		weeklyImplied := implied / hAdj
		s.Weekly[w] = p.Delta*weeklyImplied + (1-p.Delta)*s.Weekly[w]
		if s.Weekly[w] < 0.01 {
			s.Weekly[w] = 0.01
		}
	}

	if rps > s.RPSMax {
		s.RPSMax = rps
	}
}

func (p *HoltWintersPredictor) Predict(serviceName string, stepsAhead int, now time.Time) float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.predictLocked(serviceName, stepsAhead, now)
}

func (p *HoltWintersPredictor) PredictPeak(serviceName string, horizonSteps int, now time.Time) float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	peak := 0.0
	for k := 1; k <= horizonSteps; k++ {
		pred := p.predictLocked(serviceName, k, now)
		if pred > peak {
			peak = pred
		}
	}
	return peak
}

func (p *HoltWintersPredictor) predictLocked(serviceName string, stepsAhead int, now time.Time) float64 {
	s, ok := p.states[serviceName]
	if !ok {
		return 0
	}

	futureHour := (now.Hour() + stepsAhead) % 24
	futureDayOffset := (now.Hour() + stepsAhead) / 24
	futureWeekday := (int(now.Weekday()) + futureDayOffset) % 7

	trendComponent := s.Level + float64(stepsAhead)*s.Trend
	if trendComponent <= 0 {
		return 0
	}
	return trendComponent * s.Hourly[futureHour] * s.Weekly[futureWeekday]
}

func (p *HoltWintersPredictor) TrendingUp(serviceName string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	s, ok := p.states[serviceName]
	if !ok {
		return false
	}
	return s.Trend > 0.001
}

func (p *HoltWintersPredictor) GetState(serviceName string) (*HoltWintersState, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	s, ok := p.states[serviceName]
	if !ok {
		return nil, false
	}
	cp := *s
	return &cp, true
}


