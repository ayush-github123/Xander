package aggregator

import (
	"math"
	"sort"
)

// StatCalculator computes statistics from a slice of values
type StatCalculator struct {
	values []float64
}

// NewStatCalculator creates a new statistics calculator
func NewStatCalculator(values []float64) *StatCalculator {
	vals := make([]float64, len(values))
	copy(vals, values)
	return &StatCalculator{values: vals}
}

// Avg returns the average
func (sc *StatCalculator) Avg() float64 {
	if len(sc.values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range sc.values {
		sum += v
	}
	return sum / float64(len(sc.values))
}

// Min returns the minimum value
func (sc *StatCalculator) Min() float64 {
	if len(sc.values) == 0 {
		return 0
	}
	min := sc.values[0]
	for _, v := range sc.values {
		if v < min {
			min = v
		}
	}
	return min
}

// Max returns the maximum value
func (sc *StatCalculator) Max() float64 {
	if len(sc.values) == 0 {
		return 0
	}
	max := sc.values[0]
	for _, v := range sc.values {
		if v > max {
			max = v
		}
	}
	return max
}

// P95 returns the 95th percentile
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

// MovingAvg returns the 3-point moving average
func (sc *StatCalculator) MovingAvg() float64 {
	if len(sc.values) == 0 {
		return 0
	}

	window := 3
	if len(sc.values) < window {
		window = len(sc.values)
	}

	sum := 0.0
	for i := 0; i < window; i++ {
		sum += sc.values[len(sc.values)-window+i]
	}
	return sum / float64(window)
}

// Slope calculates linear slope
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

	slope := (n*sumXY - sumX*sumY) / denominator
	return slope
}

// RateOfChange returns the percentage change
func (sc *StatCalculator) RateOfChange() float64 {
	if len(sc.values) < 2 || sc.values[0] == 0 {
		return 0
	}
	return ((sc.values[len(sc.values)-1] - sc.values[0]) / sc.values[0]) * 100
}

// BaselineDeviation returns the deviation from the first value
func (sc *StatCalculator) BaselineDeviation() float64 {
	if len(sc.values) == 0 {
		return 0
	}
	if sc.values[0] == 0 {
		return 0
	}
	return sc.values[len(sc.values)-1] - sc.values[0]
}

// CalculateStats computes all statistics
func (sc *StatCalculator) CalculateStats() map[string]float64 {
	return map[string]float64{
		"avg":                sc.Avg(),
		"min":                sc.Min(),
		"max":                sc.Max(),
		"p95":                sc.P95(),
		"moving_avg":         sc.MovingAvg(),
		"slope":              sc.Slope(),
		"rate_of_change":     sc.RateOfChange(),
		"baseline_deviation": sc.BaselineDeviation(),
	}
}
