package config

import (
	"fmt"
	"math"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/nginx/config/http"
	templates "github.com/nginxinc/nginx-kubernetes-gateway/internal/nginx/config/template"
	"github.com/nginxinc/nginx-kubernetes-gateway/internal/state"
)

func executeSplitClients(conf state.Configuration) []byte {
	t := templates.NewTemplate([]http.SplitClient{})
	splitClients := createSplitClients(conf.BackendGroups)

	return t.Execute(splitClients)
}

func createSplitClients(backendGroups []state.BackendGroup) []http.SplitClient {
	splitClients := make([]http.SplitClient, 0, len(backendGroups))

	for _, group := range backendGroups {

		distributions := createSplitClientDistributions(group)
		if distributions == nil {
			continue
		}

		splitClients = append(splitClients, http.SplitClient{
			VariableName:  convertStringToSafeVariableName(group.GroupName()),
			Distributions: distributions,
		})

	}

	return splitClients
}

func createSplitClientDistributions(group state.BackendGroup) []http.SplitClientDistribution {
	if !group.NeedsSplit() {
		return nil
	}

	backends := group.Backends

	totalWeight := int32(0)
	for _, b := range backends {
		totalWeight += b.Weight
	}

	if totalWeight == 0 {
		return []http.SplitClientDistribution{
			{
				Percent: "100",
				Value:   invalidBackendRef,
			},
		}
	}

	distributions := make([]http.SplitClientDistribution, 0, len(backends))

	// The percentage of all backends cannot exceed 100.
	availablePercentage := float64(100)

	// Iterate over all backends except the last one.
	// The last backend will get the remaining percentage.
	for i := 0; i < len(backends)-1; i++ {
		b := backends[i]

		percentage := percentOf(b.Weight, totalWeight)
		availablePercentage -= percentage

		distributions = append(distributions, http.SplitClientDistribution{
			Percent: fmt.Sprintf("%.2f", percentage),
			Value:   getSplitClientValue(b),
		})
	}

	// The last backend gets the remaining percentage.
	// This is done to guarantee that the sum of all percentages is 100.
	lastBackend := backends[len(backends)-1]

	distributions = append(distributions, http.SplitClientDistribution{
		Percent: fmt.Sprintf("%.2f", availablePercentage),
		Value:   getSplitClientValue(lastBackend),
	})

	return distributions
}

func getSplitClientValue(b state.BackendRef) string {
	if b.Valid {
		return b.Name
	}
	return invalidBackendRef
}

// percentOf returns the percentage of a weight out of a totalWeight.
// The percentage is rounded to 2 decimal places using the Floor method.
// Floor is used here in order to guarantee that the sum of all percentages does not exceed 100.
// Ex. percentOf(2, 3) = 66.66
// Ex. percentOf(800, 2000) = 40.00
func percentOf(weight, totalWeight int32) float64 {
	p := (float64(weight) * 100) / float64(totalWeight)
	return math.Floor(p*100) / 100
}
