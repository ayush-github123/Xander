package ruleengine

import "sort"

type Engine struct {
	rules []Rule
}

func NewEngine(rules ...Rule) *Engine {
	copied := make([]Rule, len(rules))
	copy(copied, rules)
	return &Engine{rules: copied}
}

func NewDefaultEngine() *Engine {
	return NewEngine(DefaultRules()...)
}

func (e *Engine) Rules() []RuleDefinition {
	definitions := make([]RuleDefinition, 0, len(e.rules))
	for _, rule := range e.rules {
		definitions = append(definitions, rule.Definition())
	}
	return definitions
}

func (e *Engine) Evaluate(features FeatureSet) []Finding {
	findings := make([]Finding, 0)
	for _, rule := range e.rules {
		def := rule.Definition()
		for _, finding := range rule.Evaluate(features) {
			if finding.RuleID == "" {
				finding.RuleID = def.ID
			}
			if finding.Name == "" {
				finding.Name = def.Name
			}
			if finding.Category == "" {
				finding.Category = def.Category
			}
			if finding.Signals == nil {
				finding.Signals = map[string]float64{}
			}
			findings = append(findings, finding)
		}
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Confidence == findings[j].Confidence {
			return findings[i].RuleID < findings[j].RuleID
		}
		return findings[i].Confidence > findings[j].Confidence
	})

	return findings
}
