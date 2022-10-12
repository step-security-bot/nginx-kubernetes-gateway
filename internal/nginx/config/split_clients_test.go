package config

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/types"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/nginx/config/http"
	"github.com/nginxinc/nginx-kubernetes-gateway/internal/state"
)

func TestExecuteSplitClients(t *testing.T) {
	bg1 := state.BackendGroup{
		Source:  types.NamespacedName{Namespace: "test", Name: "hr"},
		RuleIdx: 0,
		Backends: []state.BackendRef{
			{Name: "test1", Valid: true, Weight: 1},
			{Name: "test2", Valid: true, Weight: 1},
		},
	}

	bg2 := state.BackendGroup{
		Source:  types.NamespacedName{Namespace: "test", Name: "no-split"},
		RuleIdx: 1,
		Backends: []state.BackendRef{
			{Name: "no-split", Valid: true, Weight: 1},
		},
	}

	bg3 := state.BackendGroup{
		Source:  types.NamespacedName{Namespace: "test", Name: "hr"},
		RuleIdx: 1,
		Backends: []state.BackendRef{
			{Name: "test3", Valid: true, Weight: 1},
			{Name: "test4", Valid: true, Weight: 1},
		},
	}

	backends := []state.BackendGroup{
		bg1,
		bg2,
		bg3,
	}

	expectedSubStrings := []string{
		"split_clients $request_id $test_hr_rule0",
		"split_clients $request_id $test_hr_rule1",
		"50.00% test1;",
		"50.00% test2;",
		"50.00% test3;",
		"50.00% test4;",
	}

	notExpectedSubString := "no-split"

	sc := string(executeSplitClients(state.Configuration{BackendGroups: backends}))

	for _, expSubString := range expectedSubStrings {
		if !strings.Contains(sc, expSubString) {
			t.Errorf(
				"executeSplitClients() did not generate split clients with substring %q. Got: %v",
				expSubString,
				sc,
			)
		}
	}

	if strings.Contains(sc, notExpectedSubString) {
		t.Errorf(
			"executeSplitClients() generated split clients with unexpected substring %q. Got: %v",
			notExpectedSubString,
			sc,
		)
	}
}

func TestCreateSplitClients(t *testing.T) {
	hrNoSplit := types.NamespacedName{Namespace: "test", Name: "hr-no-split"}
	hrOneSplit := types.NamespacedName{Namespace: "test", Name: "hr-one-split"}
	hrTwoSplits := types.NamespacedName{Namespace: "test", Name: "hr-two-splits"}

	createBackendGroup := func(
		sourceNsName types.NamespacedName,
		ruleIdx int,
		backends ...state.BackendRef,
	) state.BackendGroup {
		return state.BackendGroup{
			Source:   sourceNsName,
			RuleIdx:  ruleIdx,
			Backends: backends,
		}
	}
	// the following backends do not need splits
	noBackends := createBackendGroup(hrNoSplit, 0)

	oneBackend := createBackendGroup(
		hrNoSplit,
		0,
		state.BackendRef{Name: "one-backend", Valid: true, Weight: 1},
	)

	invalidBackend := createBackendGroup(
		hrNoSplit,
		0,
		state.BackendRef{Name: "invalid-backend", Valid: false, Weight: 1},
	)

	// the following backends need splits
	oneSplit := createBackendGroup(
		hrOneSplit,
		0,
		state.BackendRef{Name: "one-split-1", Valid: true, Weight: 50},
		state.BackendRef{Name: "one-split-2", Valid: true, Weight: 50},
	)

	twoSplitGroup0 := createBackendGroup(
		hrTwoSplits,
		0,
		state.BackendRef{Name: "two-split-1", Valid: true, Weight: 50},
		state.BackendRef{Name: "two-split-2", Valid: true, Weight: 50},
	)

	twoSplitGroup1 := createBackendGroup(
		hrTwoSplits,
		1,
		state.BackendRef{Name: "two-split-3", Valid: true, Weight: 50},
		state.BackendRef{Name: "two-split-4", Valid: true, Weight: 50},
		state.BackendRef{Name: "two-split-5", Valid: true, Weight: 50},
	)

	backends := []state.BackendGroup{
		noBackends,
		oneBackend,
		invalidBackend,
		oneSplit,
		twoSplitGroup0,
		twoSplitGroup1,
	}

	expSplitClients := []http.SplitClient{
		{
			VariableName: "test_hr_one_split_rule0",
			Distributions: []http.SplitClientDistribution{
				{
					Percent: "50.00",
					Value:   "one-split-1",
				},
				{
					Percent: "50.00",
					Value:   "one-split-2",
				},
			},
		},
		{
			VariableName: "test_hr_two_splits_rule0",
			Distributions: []http.SplitClientDistribution{
				{
					Percent: "50.00",
					Value:   "two-split-1",
				},
				{
					Percent: "50.00",
					Value:   "two-split-2",
				},
			},
		},
		{
			VariableName: "test_hr_two_splits_rule1",
			Distributions: []http.SplitClientDistribution{
				{
					Percent: "33.33",
					Value:   "two-split-3",
				},
				{
					Percent: "33.33",
					Value:   "two-split-4",
				},
				{
					Percent: "33.34",
					Value:   "two-split-5",
				},
			},
		},
	}

	result := createSplitClients(backends)
	if diff := cmp.Diff(expSplitClients, result); diff != "" {
		t.Errorf("createSplitClients() mismatch (-want +got):\n%s", diff)
	}
}

func TestCreateSplitClientDistributions(t *testing.T) {
	tests := []struct {
		msg              string
		backends         []state.BackendRef
		expDistributions []http.SplitClientDistribution
	}{
		{
			msg:              "no backends",
			backends:         nil,
			expDistributions: nil,
		},
		{
			msg: "one backend",
			backends: []state.BackendRef{
				{
					Name:   "one",
					Valid:  true,
					Weight: 1,
				},
			},
			expDistributions: nil,
		},
		{
			msg: "total weight 0",
			backends: []state.BackendRef{
				{
					Name:   "one",
					Valid:  true,
					Weight: 0,
				},
				{
					Name:   "two",
					Valid:  true,
					Weight: 0,
				},
			},
			expDistributions: []http.SplitClientDistribution{
				{
					Percent: "100",
					Value:   invalidBackendRef,
				},
			},
		},
		{
			msg: "two backends; equal weights that sum to 100",
			backends: []state.BackendRef{
				{
					Name:   "one",
					Valid:  true,
					Weight: 1,
				},
				{
					Name:   "two",
					Valid:  true,
					Weight: 1,
				},
			},
			expDistributions: []http.SplitClientDistribution{
				{
					Percent: "50.00",
					Value:   "one",
				},
				{
					Percent: "50.00",
					Value:   "two",
				},
			},
		},
		{
			msg: "three backends; whole percentages that sum to 100",
			backends: []state.BackendRef{
				{
					Name:   "one",
					Valid:  true,
					Weight: 20,
				},
				{
					Name:   "two",
					Valid:  true,
					Weight: 30,
				},
				{
					Name:   "three",
					Valid:  true,
					Weight: 50,
				},
			},
			expDistributions: []http.SplitClientDistribution{
				{
					Percent: "20.00",
					Value:   "one",
				},
				{
					Percent: "30.00",
					Value:   "two",
				},
				{
					Percent: "50.00",
					Value:   "three",
				},
			},
		},
		{
			msg: "three backends; whole percentages that sum to less than 100",
			backends: []state.BackendRef{
				{
					Name:   "one",
					Valid:  true,
					Weight: 3,
				},
				{
					Name:   "two",
					Valid:  true,
					Weight: 3,
				},
				{
					Name:   "three",
					Valid:  true,
					Weight: 3,
				},
			},
			expDistributions: []http.SplitClientDistribution{
				{
					Percent: "33.33",
					Value:   "one",
				},
				{
					Percent: "33.33",
					Value:   "two",
				},
				{
					Percent: "33.34", // the last backend gets the remainder.
					Value:   "three",
				},
			},
		},
	}

	for _, test := range tests {
		result := createSplitClientDistributions(state.BackendGroup{Backends: test.backends})
		if diff := cmp.Diff(test.expDistributions, result); diff != "" {
			t.Errorf("createSplitClientDistributions() mismatch for %q (-want +got):\n%s", test.msg, diff)
		}
	}
}

func TestGetSplitClientValue(t *testing.T) {
	tests := []struct {
		msg      string
		backend  state.BackendRef
		expValue string
	}{
		{
			msg: "valid backend",
			backend: state.BackendRef{
				Name:  "valid",
				Valid: true,
			},
			expValue: "valid",
		},
		{
			msg: "invalid backend",
			backend: state.BackendRef{
				Name:  "invalid",
				Valid: false,
			},
			expValue: invalidBackendRef,
		},
	}

	for _, test := range tests {
		result := getSplitClientValue(test.backend)
		if result != test.expValue {
			t.Errorf(
				"getSplitClientValue() mismatch for %q; expected %s, got %s",
				test.msg, test.expValue, result,
			)
		}
	}
}

func TestPercentOf(t *testing.T) {
	tests := []struct {
		msg         string
		weight      int32
		totalWeight int32
		expPercent  float64
	}{
		{
			msg:         "50/100",
			weight:      50,
			totalWeight: 100,
			expPercent:  50,
		},
		{
			msg:         "2000/4000",
			weight:      2000,
			totalWeight: 4000,
			expPercent:  50,
		},
		{
			msg:         "100/100",
			weight:      100,
			totalWeight: 100,
			expPercent:  100,
		},
		{
			msg:         "5/5",
			weight:      5,
			totalWeight: 5,
			expPercent:  100,
		},
		{
			msg:         "0/8000",
			weight:      0,
			totalWeight: 8000,
			expPercent:  0,
		},
		{
			msg:         "2/3",
			weight:      2,
			totalWeight: 3,
			expPercent:  66.66,
		},
		{
			msg:         "4/15",
			weight:      4,
			totalWeight: 15,
			expPercent:  26.66,
		},
		{
			msg:         "800/2000",
			weight:      800,
			totalWeight: 2000,
			expPercent:  40,
		},
		{
			msg:         "300/2400",
			weight:      300,
			totalWeight: 2400,
			expPercent:  12.5,
		},
	}

	for _, test := range tests {
		percent := percentOf(test.weight, test.totalWeight)
		if percent != test.expPercent {
			t.Errorf(
				"percentOf() mismatch for test %q; expected %f, got %f",
				test.msg, test.expPercent, percent,
			)
		}
	}
}
