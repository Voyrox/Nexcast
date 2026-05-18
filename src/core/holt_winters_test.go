package nextcast

import (
	"math"
	"sync"
	"testing"
	"time"
)

func TestNewHoltWintersPredictor(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6)
	if p == nil {
		t.Fatal("expected non-nil predictor")
	}
	if p.Alpha != 0.3 || p.Beta != 0.1 || p.Gamma != 0.1 || p.Delta != 0.05 || p.Horizon != 6 {
		t.Fatal("unexpected parameter values")
	}
	if len(p.states) != 0 {
		t.Fatal("expected empty states map")
	}
}

func TestPredictNoState(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6)
	now := time.Now()

	if pred := p.Predict("nonexistent", 1, now); pred != 0 {
		t.Fatalf("expected 0, got %f", pred)
	}
	if peak := p.PredictPeak("nonexistent", 6, now); peak != 0 {
		t.Fatalf("expected 0, got %f", peak)
	}
	if state, ok := p.GetState("nonexistent"); ok {
		t.Fatal("expected false for nonexistent service")
	} else if state != nil {
		t.Fatal("expected nil state")
	}
}

func TestFirstUpdateCreatesState(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6)
	now := time.Now()

	p.Update("api", 100.0, now)

	state, ok := p.GetState("api")
	if !ok {
		t.Fatal("expected state after update")
	}
	if state.Level != 100.0 {
		t.Fatalf("expected level 100, got %f", state.Level)
	}
	if state.Trend != 0 {
		t.Fatalf("expected trend 0, got %f", state.Trend)
	}
	if state.RPSMax != 100.0 {
		t.Fatalf("expected RPSMax 100, got %f", state.RPSMax)
	}
}

func TestConstantRPS(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6)
	now := time.Now()

	for i := 0; i < 100; i++ {
		ts := now.Add(time.Duration(i) * time.Hour)
		p.Update("api", 100.0, ts)
	}

	pred := p.Predict("api", 1, now.Add(100*time.Hour))
	if pred < 90 || pred > 110 {
		t.Fatalf("expected ~100, got %f", pred)
	}

	peak := p.PredictPeak("api", 6, now.Add(100*time.Hour))
	if peak < 90 || peak > 110 {
		t.Fatalf("expected peak ~100, got %f", peak)
	}
}

func TestTrendingUp(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6)
	now := time.Now()

	if p.TrendingUp("unknown") {
		t.Fatal("expected false for unknown service")
	}

	for i := 0; i < 50; i++ {
		rps := 50.0 + float64(i)*2.0
		ts := now.Add(time.Duration(i) * time.Hour)
		p.Update("api", rps, ts)
	}

	if !p.TrendingUp("api") {
		t.Fatal("expected trending up")
	}

	state, _ := p.GetState("api")
	if state.Trend <= 0 {
		t.Fatalf("expected positive trend, got %f", state.Trend)
	}
}

func TestMultiService(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6)
	now := time.Now()

	for i := 0; i < 50; i++ {
		ts := now.Add(time.Duration(i) * time.Hour)
		p.Update("api", 100.0, ts)
		p.Update("worker", 50.0, ts)
	}

	apiState, _ := p.GetState("api")
	workerState, _ := p.GetState("worker")

	if apiState.Level < 90 || apiState.Level > 110 {
		t.Fatalf("api level expected ~100, got %f", apiState.Level)
	}
	if workerState.Level < 45 || workerState.Level > 55 {
		t.Fatalf("worker level expected ~50, got %f", workerState.Level)
	}
}

func TestSpikeResetsRPSMax(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6)
	now := time.Now()

	for i := 0; i < 10; i++ {
		ts := now.Add(time.Duration(i) * time.Hour)
		p.Update("api", 100.0, ts)
	}

	state, _ := p.GetState("api")
	if state.RPSMax != 100 {
		t.Fatalf("expected RPSMax 100, got %f", state.RPSMax)
	}

	p.Update("api", 500.0, now.Add(10*time.Hour))
	state, _ = p.GetState("api")
	if state.RPSMax != 500 {
		t.Fatalf("expected RPSMax 500, got %f", state.RPSMax)
	}
}

func TestHourlySeasonality(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.2, 0.3, 0.2, 6)

	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	for day := 0; day < 21; day++ {
		for hour := 0; hour < 24; hour++ {
			ts := baseTime.AddDate(0, 0, day).Add(time.Duration(hour) * time.Hour)
			var rps float64
			if hour >= 8 && hour <= 18 {
				rps = 200.0
			} else {
				rps = 50.0
			}
			p.Update("api", rps, ts)
		}
	}

	state, _ := p.GetState("api")

	for hour := 8; hour <= 18; hour++ {
		if state.Hourly[hour] < 1.5 {
			t.Fatalf("expected hour %d factor >= 1.5, got %.3f", hour, state.Hourly[hour])
		}
	}
	for hour := 0; hour < 8; hour++ {
		if state.Hourly[hour] > 1.0 {
			t.Fatalf("expected hour %d factor <= 1.0, got %.3f", hour, state.Hourly[hour])
		}
	}
}

func TestWeeklySeasonality(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.2, 0.3, 0.3, 6)

	baseTime := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)

	for week := 0; week < 16; week++ {
		for day := 0; day < 7; day++ {
			for hour := 0; hour < 24; hour += 4 {
				ts := baseTime.AddDate(0, 0, week*7+day).Add(time.Duration(hour) * time.Hour)
				var rps float64
				if day < 5 {
					rps = 150.0
				} else {
					rps = 50.0
				}
				p.Update("api", rps, ts)
			}
		}
	}

	state, _ := p.GetState("api")

	weekdayAvg := 0.0
	for day := 0; day < 5; day++ {
		weekdayAvg += state.Weekly[day]
	}
	weekdayAvg /= 5

	weekendAvg := 0.0
	for day := 5; day < 7; day++ {
		weekendAvg += state.Weekly[day]
	}
	weekendAvg /= 2

	if weekdayAvg <= weekendAvg {
		t.Fatalf("expected weekday avg (%.3f) > weekend avg (%.3f)", weekdayAvg, weekendAvg)
	}
	if weekdayAvg < 1.1 {
		t.Fatalf("expected weekday avg >= 1.1, got %.3f", weekdayAvg)
	}
	if weekendAvg > 0.9 {
		t.Fatalf("expected weekend avg <= 0.9, got %.3f", weekendAvg)
	}
}

func TestPredictPeakReturnsMax(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6)
	now := time.Now()

	for i := 0; i < 50; i++ {
		ts := now.Add(time.Duration(i) * time.Hour)
		rps := 100.0 + 50.0*math.Sin(float64(i)*0.5)
		p.Update("api", rps, ts)
	}

	peak := p.PredictPeak("api", 6, now.Add(50*time.Hour))
	single := p.Predict("api", 3, now.Add(50*time.Hour))

	if peak < single {
		t.Fatalf("peak (%f) should be >= single prediction (%f)", peak, single)
	}
}

func TestRecordRPSNoLongerExists(t *testing.T) {
	n := &Nexcast{
		hwPredictor: NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6),
	}

	n.hwPredictor.Update("api", 100.0, time.Now())

	state, ok := n.hwPredictor.GetState("api")
	if !ok {
		t.Fatal("expected state to exist")
	}
	if state.Level != 100.0 {
		t.Fatalf("expected level 100, got %f", state.Level)
	}
}

func TestPredictDemandAfterMultipleUpdates(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.2, 0.2, 0.1, 6)
	now := time.Now()

	for i := 0; i < 168; i++ {
		ts := now.Add(time.Duration(i) * time.Hour)
		hour := ts.Hour()
		rps := 80.0 + 40.0*math.Sin(float64(hour-8)/24.0*2*math.Pi)
		p.Update("api", rps, ts)
	}

	pred := p.Predict("api", 1, now.Add(168*time.Hour))
	if pred < 40 || pred > 130 {
		t.Fatalf("prediction %f out of expected range [40, 130]", pred)
	}
}

func TestConcurrentAccess(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6)
	now := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := "api"
			if id%2 == 0 {
				name = "worker"
			}
			for j := 0; j < 20; j++ {
				ts := now.Add(time.Duration(j) * time.Hour)
				p.Update(name, float64(50+id*10+j), ts)
				p.Predict(name, 1, ts)
				p.PredictPeak(name, 3, ts)
				p.TrendingUp(name)
				p.GetState(name)
			}
		}(i)
	}
	wg.Wait()

	apiState, apiOk := p.GetState("api")
	workerState, workerOk := p.GetState("worker")
	if !apiOk {
		t.Fatal("missing api state after concurrent access")
	}
	if !workerOk {
		t.Fatal("missing worker state after concurrent access")
	}
	if apiState.Level <= 0 {
		t.Fatal("api level should be positive")
	}
	if workerState.Level <= 0 {
		t.Fatal("worker level should be positive")
	}
}

func TestPredictReturnsCopyState(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6)
	now := time.Now()

	for i := 0; i < 10; i++ {
		p.Update("api", 100.0, now.Add(time.Duration(i)*time.Hour))
	}

	state1, _ := p.GetState("api")
	state2, _ := p.GetState("api")

	state1.Level = 999
	if state2.Level == 999 {
		t.Fatal("GetState should return a copy, not a reference")
	}
}

func TestColdStartFallback(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6)
	now := time.Now()

	if pred := p.Predict("new-service", 1, now); pred != 0 {
		t.Fatalf("expected 0 for cold start, got %f", pred)
	}
	if peak := p.PredictPeak("new-service", 6, now); peak != 0 {
		t.Fatalf("expected 0 for cold start, got %f", peak)
	}

	p.Update("new-service", 75.0, now)

	pred := p.Predict("new-service", 1, now)
	if pred <= 0 {
		t.Fatalf("expected positive prediction after update, got %f", pred)
	}
}

func TestSeedFromHistory(t *testing.T) {
	n := &Nexcast{
		hwPredictor: NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6),
	}

	now := time.Now()
	for i := 0; i < 48; i++ {
		ts := now.Add(time.Duration(-48+i) * time.Hour)
		rps := 100.0
		if ts.Hour() >= 8 && ts.Hour() <= 18 {
			rps = 200.0
		}
		n.hwPredictor.Update("api", rps, ts)
	}

	state, ok := n.hwPredictor.GetState("api")
	if !ok {
		t.Fatal("expected state after seeding")
	}
	if state.Level <= 0 {
		t.Fatalf("expected positive level, got %f", state.Level)
	}
	pred := n.hwPredictor.Predict("api", 1, now)
	if pred <= 0 {
		t.Fatalf("expected positive prediction after seeding, got %f", pred)
	}
}

func TestPredictPeakDifferentHorizons(t *testing.T) {
	p := NewHoltWintersPredictor(0.3, 0.1, 0.1, 0.05, 6)
	now := time.Now()

	for i := 0; i < 48; i++ {
		p.Update("api", 100.0, now.Add(time.Duration(i)*time.Hour))
	}

	peak1 := p.PredictPeak("api", 1, now.Add(48*time.Hour))
	peak6 := p.PredictPeak("api", 6, now.Add(48*time.Hour))

	if peak1 <= 0 {
		t.Fatalf("expected positive peak1, got %f", peak1)
	}
	if peak6 <= 0 {
		t.Fatalf("expected positive peak6, got %f", peak6)
	}
}
