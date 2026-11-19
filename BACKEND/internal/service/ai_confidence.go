package service

import (
	"encoding/json"
	"math"
	"os"
	"strconv"
)

type confidenceBin struct {
	Correct int `json:"correct"`
	Total   int `json:"total"`
}

// Each bin contains outcomes from labeled, held-out examples for this model and task.
type confidenceCalibration [10]confidenceBin

func calibrationFromEnvironment(name string) confidenceCalibration {
	var bins confidenceCalibration
	var configured []confidenceBin
	if json.Unmarshal([]byte(os.Getenv(name)), &configured) == nil && len(configured) == len(bins) {
		copy(bins[:], configured)
	}
	return bins
}

// Use the lower 95% Wilson bound as the decision score to account for sample size.
func (bins confidenceCalibration) probability(raw float64) (float64, float64) {
	if math.IsNaN(raw) || raw < 0 || raw > 1 {
		return 0, 0
	}
	bin := bins[min(int(raw*10), 9)]
	if bin.Total < 20 || bin.Correct < 0 || bin.Correct > bin.Total {
		return 0, 0
	}
	n := float64(bin.Total)
	p := float64(bin.Correct) / n
	const z = 1.96
	bound := (p + z*z/(2*n) - z*math.Sqrt((p*(1-p)+z*z/(4*n))/n)) / (1 + z*z/n)
	return p, bound
}

func confidenceThreshold(name string, fallback float64) float64 {
	value, err := strconv.ParseFloat(os.Getenv(name), 64)
	if err == nil && value >= .7 && value <= 1 {
		return value
	}
	return fallback
}
