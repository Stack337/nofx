package ai500

import (
	"math"
	"sort"
)

type EvaluationReport struct {
	Count               int     `json:"count"`
	PrecisionAtK        float64 `json:"precision_at_k"`
	DirectionalAccuracy float64 `json:"directional_accuracy"`
	AverageReturnPct    float64 `json:"average_return_pct"`
	MaxDrawdownPct      float64 `json:"max_drawdown_pct"`
	ScoreStability      float64 `json:"score_stability"`
}

type evaluationItem struct {
	observation ScoreObservation
	label       OutcomeLabel
	adjusted    float64
}

func Evaluate(observations []ScoreObservation, labels []OutcomeLabel, topK int) EvaluationReport {
	byID := make(map[string]ScoreObservation, len(observations))
	for _, observation := range observations {
		if observation.ID != "" {
			byID[observation.ID] = observation
		}
	}
	items := make([]evaluationItem, 0, len(labels))
	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		observation, exists := byID[label.ObservationID]
		key := label.ObservationID + "\x00" + label.Horizon
		if !exists {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		adjusted := label.ReturnPct
		if observation.ShortScore > observation.LongScore {
			adjusted = -adjusted
		}
		items = append(items, evaluationItem{observation: observation, label: label, adjusted: adjusted})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].observation.Score != items[j].observation.Score {
			return items[i].observation.Score > items[j].observation.Score
		}
		if items[i].observation.ID != items[j].observation.ID {
			return items[i].observation.ID < items[j].observation.ID
		}
		return items[i].label.Horizon < items[j].label.Horizon
	})

	report := EvaluationReport{Count: len(items), ScoreStability: scoreStability(observations)}
	if len(items) == 0 {
		return report
	}
	wins := 0
	totalReturn := 0.0
	report.MaxDrawdownPct = items[0].label.MaxDrawdownPct
	for _, item := range items {
		if item.adjusted > 0 {
			wins++
		}
		totalReturn += item.adjusted
		if item.label.MaxDrawdownPct < report.MaxDrawdownPct {
			report.MaxDrawdownPct = item.label.MaxDrawdownPct
		}
	}
	report.DirectionalAccuracy = float64(wins) / float64(len(items)) * 100
	report.AverageReturnPct = totalReturn / float64(len(items))

	if topK > len(items) {
		topK = len(items)
	}
	if topK > 0 {
		topWins := 0
		for _, item := range items[:topK] {
			if item.adjusted > 0 {
				topWins++
			}
		}
		report.PrecisionAtK = float64(topWins) / float64(topK) * 100
	}
	return report
}

func scoreStability(observations []ScoreObservation) float64 {
	bySymbol := make(map[string][]ScoreObservation)
	for _, observation := range observations {
		bySymbol[observation.Symbol] = append(bySymbol[observation.Symbol], observation)
	}
	totalDelta := 0.0
	transitions := 0
	for _, items := range bySymbol {
		sort.Slice(items, func(i, j int) bool {
			if !items[i].FeatureTimestamp.Equal(items[j].FeatureTimestamp) {
				return items[i].FeatureTimestamp.Before(items[j].FeatureTimestamp)
			}
			return items[i].ID < items[j].ID
		})
		for index := 1; index < len(items); index++ {
			totalDelta += math.Abs(items[index].Score - items[index-1].Score)
			transitions++
		}
	}
	if transitions == 0 {
		return 100
	}
	return math.Max(0, 100-totalDelta/float64(transitions))
}
