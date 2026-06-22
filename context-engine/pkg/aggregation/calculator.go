package aggregation

import (
	"math"
	"sort"
)

type StatCalculator struct {
	values []float64
}

func NewStatCalculator(values []float64) *StatCalculator {
	copied := make([]float64, len(values))
	copy(copied, values)
	return &StatCalculator{values: copied}
}

func (sc *StatCalculator) Avg() float64 {
	if len(sc.values) == 0 {
		return 0
	}
	sum := 0.0
	for _, value := range sc.values {
		sum += value
	}
	return sum / float64(len(sc.values))
}

func (sc *StatCalculator) Min() float64 {
	if len(sc.values) == 0 {
		return 0
	}
	minimum := sc.values[0]
	for _, value := range sc.values {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func (sc *StatCalculator) Max() float64 {
	if len(sc.values) == 0 {
		return 0
	}
	maximum := sc.values[0]
	for _, value := range sc.values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func (sc *StatCalculator) P95() float64 {
	if len(sc.values) == 0 {
		return 0
	}
	sorted := make([]float64, len(sc.values))
	copy(sorted, sc.values)
	sort.Float64s(sorted)

	index := int(math.Ceil(float64(len(sorted))*0.95)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func (sc *StatCalculator) MovingAvg() float64 {
	if len(sc.values) == 0 {
		return 0
	}
	window := 3
	if len(sc.values) < window {
		window = len(sc.values)
	}

	sum := 0.0
	for i := len(sc.values) - window; i < len(sc.values); i++ {
		sum += sc.values[i]
	}
	return sum / float64(window)
}

func (sc *StatCalculator) Slope() float64 {
	if len(sc.values) < 2 {
		return 0
	}

	points := len(sc.values)
	if points > 5 {
		points = 5
	}
	startIdx := len(sc.values) - points

	n := float64(points)
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumX2 := 0.0

	for i := 0; i < points; i++ {
		x := float64(i)
		y := sc.values[startIdx+i]
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	denominator := n*sumX2 - sumX*sumX
	if math.Abs(denominator) < 1e-10 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / denominator
}

func (sc *StatCalculator) RateOfChange() float64 {
	if len(sc.values) < 2 || sc.values[0] == 0 {
		return 0
	}
	return ((sc.values[len(sc.values)-1] - sc.values[0]) / sc.values[0]) * 100
}

func (sc *StatCalculator) BaselineDeviation() float64 {
	if len(sc.values) == 0 {
		return 0
	}
	return sc.values[len(sc.values)-1] - sc.values[0]
}

func (sc *StatCalculator) CalculateStats() MetricStatistics {
	return MetricStatistics{
		Avg:               sc.Avg(),
		Min:               sc.Min(),
		Max:               sc.Max(),
		P95:               sc.P95(),
		MovingAvg:         sc.MovingAvg(),
		Slope:             sc.Slope(),
		RateOfChange:      sc.RateOfChange(),
		BaselineDeviation: sc.BaselineDeviation(),
	}
}
