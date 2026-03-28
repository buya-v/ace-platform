package types

import "testing"

func TestSideString(t *testing.T) {
	tests := []struct {
		side Side
		want string
	}{
		{SideBuy, "BUY"},
		{SideSell, "SELL"},
	}
	for _, tt := range tests {
		if got := tt.side.String(); got != tt.want {
			t.Errorf("Side(%d).String() = %s, want %s", tt.side, got, tt.want)
		}
	}
}

func TestSideOpposite(t *testing.T) {
	if SideBuy.Opposite() != SideSell {
		t.Error("SideBuy.Opposite() should be SideSell")
	}
	if SideSell.Opposite() != SideBuy {
		t.Error("SideSell.Opposite() should be SideBuy")
	}
}

func TestClearingStatusString(t *testing.T) {
	tests := []struct {
		status ClearingStatus
		want   string
	}{
		{ClearingStatusPending, "PENDING"},
		{ClearingStatusNovated, "NOVATED"},
		{ClearingStatusNetted, "NETTED"},
		{ClearingStatusSettled, "SETTLED"},
		{ClearingStatusRejected, "REJECTED"},
		{ClearingStatus(99), "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestPositionIsLong(t *testing.T) {
	p := Position{NetQuantity: 10}
	if !p.IsLong() {
		t.Error("expected IsLong true for positive net")
	}
	if p.IsShort() {
		t.Error("expected IsShort false for positive net")
	}
	if p.IsFlat() {
		t.Error("expected IsFlat false for positive net")
	}
}

func TestPositionIsShort(t *testing.T) {
	p := Position{NetQuantity: -5}
	if p.IsLong() {
		t.Error("expected IsLong false for negative net")
	}
	if !p.IsShort() {
		t.Error("expected IsShort true for negative net")
	}
	if p.IsFlat() {
		t.Error("expected IsFlat false for negative net")
	}
}

func TestPositionIsFlat(t *testing.T) {
	p := Position{NetQuantity: 0}
	if p.IsLong() {
		t.Error("expected IsLong false for zero net")
	}
	if p.IsShort() {
		t.Error("expected IsShort false for zero net")
	}
	if !p.IsFlat() {
		t.Error("expected IsFlat true for zero net")
	}
}

func TestNettingEfficiency(t *testing.T) {
	tests := []struct {
		name      string
		long      uint64
		short     uint64
		net       int64
		wantLow   float64
		wantHigh  float64
	}{
		{"full offset", 100, 100, 0, 100.0, 100.0},
		{"no offset buy only", 100, 0, 100, 0.0, 0.0},
		{"no offset sell only", 0, 50, -50, 0.0, 0.0},
		{"partial offset", 100, 80, 20, 88.0, 89.0},
		{"zero gross", 0, 0, 0, 0.0, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &NettingResult{
				GrossLongQty:  tt.long,
				GrossShortQty: tt.short,
				NetQuantity:   tt.net,
			}
			eff := r.NettingEfficiency()
			if eff < tt.wantLow || eff > tt.wantHigh {
				t.Errorf("efficiency = %.2f, want [%.2f, %.2f]", eff, tt.wantLow, tt.wantHigh)
			}
		})
	}
}

func TestAbs64(t *testing.T) {
	tests := []struct {
		input int64
		want  int64
	}{
		{0, 0},
		{5, 5},
		{-5, 5},
		{-1, 1},
		{100, 100},
	}
	for _, tt := range tests {
		if got := abs64(tt.input); got != tt.want {
			t.Errorf("abs64(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
