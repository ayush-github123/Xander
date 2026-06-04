package analyzer

import (
	"math"
)

// CorrelationAnalyzer finds relationships between containers
type CorrelationAnalyzer struct{}

// NewCorrelationAnalyzer creates a new correlation analyzer
func NewCorrelationAnalyzer() *CorrelationAnalyzer {
	return &CorrelationAnalyzer{}
}

// FindRelatedContainers identifies related containers based on naming patterns and namespaces
func (ca *CorrelationAnalyzer) FindRelatedContainers(currentIdentity string, allIdentities []string) []string {
	var related []string

	parts := extractIdentityParts(currentIdentity)
	if len(parts) != 3 {
		return related
	}

	namespace := parts[0]
	podName := parts[1]

	for _, identity := range allIdentities {
		if identity == currentIdentity {
			continue
		}

		otherParts := extractIdentityParts(identity)
		if len(otherParts) != 3 {
			continue
		}

		// Same namespace or pod = related
		if otherParts[0] == namespace || otherParts[1] == podName {
			related = append(related, identity)
		}

		// Check for service dependencies in naming
		if isServicePair(podName, otherParts[1]) {
			related = append(related, identity)
		}
	}

	if len(related) > 10 {
		related = related[:10] // limit to top 10
	}

	return related
}

func extractIdentityParts(identity string) []string {
	parts := []string{}
	i := 0
	j := 0
	for i < len(identity) {
		if identity[i] == '/' {
			parts = append(parts, identity[j:i])
			j = i + 1
		}
		i++
	}
	if j < len(identity) {
		parts = append(parts, identity[j:])
	}
	return parts
}

func isServicePair(pod1, pod2 string) bool {
	// Check if pods are related services
	return false
}

// CalculatePearsonCorrelation computes correlation between two metric series
func (ca *CorrelationAnalyzer) CalculatePearsonCorrelation(series1, series2 []float64) float64 {
	if len(series1) != len(series2) || len(series1) == 0 {
		return 0
	}

	mean1 := calculateMean(series1)
	mean2 := calculateMean(series2)

	var numerator float64
	var denom1 float64
	var denom2 float64

	for i := 0; i < len(series1); i++ {
		diff1 := series1[i] - mean1
		diff2 := series2[i] - mean2
		numerator += diff1 * diff2
		denom1 += diff1 * diff1
		denom2 += diff2 * diff2
	}

	if denom1 == 0 || denom2 == 0 {
		return 0
	}

	return numerator / math.Sqrt(denom1*denom2)
}

func calculateMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
