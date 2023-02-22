package config

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/types"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/nginx/config/http"
	"github.com/nginxinc/nginx-kubernetes-gateway/internal/state/dataplane"
	"github.com/nginxinc/nginx-kubernetes-gateway/internal/state/graph"
)

func TestExecuteSplitClients(t *testing.T) {
	bg1 := graph.BackendGroup{
		Source:  types.NamespacedName{Namespace: "test", Name: "hr"},
		RuleIdx: 0,
		Backends: []graph.BackendRef{
			{Name: "test1", Valid: true, Weight: 1},
			{Name: "test2", Valid: true, Weight: 1},
		},
	}

	bg2 := graph.BackendGroup{
		Source:  types.NamespacedName{Namespace: "test", Name: "no-split"},
		RuleIdx: 1,
		Backends: []graph.BackendRef{
			{Name: "no-split", Valid: true, Weight: 1},
		},
	}

	bg3 := graph.BackendGroup{
		Source:  types.NamespacedName{Namespace: "test", Name: "hr"},
		RuleIdx: 1,
		Backends: []graph.BackendRef{
			{Name: "test3", Valid: true, Weight: 1},
			{Name: "test4", Valid: true, Weight: 1},
		},
	}

	tests := []struct {
		msg           string
		backendGroups []graph.BackendGroup
		expStrings    []string
		notExpStrings []string
	}{
		{
			msg: "non-zero weights",
			backendGroups: []graph.BackendGroup{
				bg1,
				bg2,
				bg3,
			},
			expStrings: []string{
				"split_clients $request_id $test__hr_rule0",
				"split_clients $request_id $test__hr_rule1",
				"50.00% test1;",
				"50.00% test2;",
				"50.00% test3;",
				"50.00% test4;",
			},
			notExpStrings: []string{"no-split", "#"},
		},
		{
			msg: "zero weight",
			backendGroups: []graph.BackendGroup{
				{
					Source:  types.NamespacedName{Namespace: "test", Name: "zero-percent"},
					RuleIdx: 0,
					Backends: []graph.BackendRef{
						{Name: "non-zero", Valid: true, Weight: 1},
						{Name: "zero", Valid: true, Weight: 0},
					},
				},
			},
			expStrings: []string{
				"split_clients $request_id $test__zero_percent_rule0",
				"100.00% non-zero;",
				"# 0.00% zero;",
			},
			notExpStrings: nil,
		},
		{
			msg: "no split clients",
			backendGroups: []graph.BackendGroup{
				{
					Source:  types.NamespacedName{Namespace: "test", Name: "single-backend-route"},
					RuleIdx: 0,
					Backends: []graph.BackendRef{
						{Name: "single-backend", Valid: true, Weight: 1},
					},
				},
			},
			expStrings:    []string{},
			notExpStrings: []string{"split_clients"},
		},
	}

	for _, test := range tests {
		sc := string(executeSplitClients(dataplane.Configuration{BackendGroups: test.backendGroups}))

		for _, expSubString := range test.expStrings {
			if !strings.Contains(sc, expSubString) {
				t.Errorf(
					"executeSplitClients() did not generate split clients with substring %q for test %q. Got: %v",
					expSubString,
					test.msg,
					sc,
				)
			}
		}

		for _, notExpString := range test.notExpStrings {
			if strings.Contains(sc, notExpString) {
				t.Errorf(
					"executeSplitClients() generated split clients with unexpected substring %q for test %q. Got: %v",
					notExpString,
					test.msg,
					sc,
				)
			}
		}
	}
}

func TestCreateSplitClients(t *testing.T) {
	hrNoSplit := types.NamespacedName{Namespace: "test", Name: "hr-no-split"}
	hrOneSplit := types.NamespacedName{Namespace: "test", Name: "hr-one-split"}
	hrTwoSplits := types.NamespacedName{Namespace: "test", Name: "hr-two-splits"}

	createBackendGroup := func(
		sourceNsName types.NamespacedName,
		ruleIdx int,
		backends ...graph.BackendRef,
	) graph.BackendGroup {
		return graph.BackendGroup{
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
		graph.BackendRef{Name: "one-backend", Valid: true, Weight: 1},
	)

	invalidBackend := createBackendGroup(
		hrNoSplit,
		0,
		graph.BackendRef{Name: "invalid-backend", Valid: false, Weight: 1},
	)

	// the following backends need splits
	oneSplit := createBackendGroup(
		hrOneSplit,
		0,
		graph.BackendRef{Name: "one-split-1", Valid: true, Weight: 50},
		graph.BackendRef{Name: "one-split-2", Valid: true, Weight: 50},
	)

	twoSplitGroup0 := createBackendGroup(
		hrTwoSplits,
		0,
		graph.BackendRef{Name: "two-split-1", Valid: true, Weight: 50},
		graph.BackendRef{Name: "two-split-2", Valid: true, Weight: 50},
	)

	twoSplitGroup1 := createBackendGroup(
		hrTwoSplits,
		1,
		graph.BackendRef{Name: "two-split-3", Valid: true, Weight: 50},
		graph.BackendRef{Name: "two-split-4", Valid: true, Weight: 50},
		graph.BackendRef{Name: "two-split-5", Valid: true, Weight: 50},
	)

	tests := []struct {
		msg             string
		backendGroups   []graph.BackendGroup
		expSplitClients []http.SplitClient
	}{
		{
			msg: "normal case",
			backendGroups: []graph.BackendGroup{
				noBackends,
				oneBackend,
				invalidBackend,
				oneSplit,
				twoSplitGroup0,
				twoSplitGroup1,
			},
			expSplitClients: []http.SplitClient{
				{
					VariableName: "test__hr_one_split_rule0",
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
					VariableName: "test__hr_two_splits_rule0",
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
					VariableName: "test__hr_two_splits_rule1",
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
			},
		},
		{
			msg: "no split clients are needed",
			backendGroups: []graph.BackendGroup{
				noBackends,
				oneBackend,
			},
			expSplitClients: nil,
		},
	}

	for _, test := range tests {
		result := createSplitClients(test.backendGroups)
		if diff := cmp.Diff(test.expSplitClients, result); diff != "" {
			t.Errorf("createSplitClients() mismatch for %q (-want +got):\n%s", test.msg, diff)
		}
	}
}

func TestCreateSplitClientDistributions(t *testing.T) {
	tests := []struct {
		msg              string
		backends         []graph.BackendRef
		expDistributions []http.SplitClientDistribution
	}{
		{
			msg:              "no backends",
			backends:         nil,
			expDistributions: nil,
		},
		{
			msg: "one backend",
			backends: []graph.BackendRef{
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
			backends: []graph.BackendRef{
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
			backends: []graph.BackendRef{
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
			backends: []graph.BackendRef{
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
			backends: []graph.BackendRef{
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
		result := createSplitClientDistributions(graph.BackendGroup{Backends: test.backends})
		if diff := cmp.Diff(test.expDistributions, result); diff != "" {
			t.Errorf("createSplitClientDistributions() mismatch for %q (-want +got):\n%s", test.msg, diff)
		}
	}
}

func TestGetSplitClientValue(t *testing.T) {
	tests := []struct {
		msg      string
		expValue string
		backend  graph.BackendRef
	}{
		{
			msg: "valid backend",
			backend: graph.BackendRef{
				Name:  "valid",
				Valid: true,
			},
			expValue: "valid",
		},
		{
			msg: "invalid backend",
			backend: graph.BackendRef{
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

func TestBackendGroupNeedsSplit(t *testing.T) {
	tests := []struct {
		msg      string
		backends []graph.BackendRef
		expSplit bool
	}{
		{
			msg:      "empty backends",
			backends: []graph.BackendRef{},
			expSplit: false,
		},
		{
			msg:      "nil backends",
			backends: nil,
			expSplit: false,
		},
		{
			msg: "one valid backend",
			backends: []graph.BackendRef{
				{
					Name:   "backend1",
					Valid:  true,
					Weight: 1,
				},
			},
			expSplit: false,
		},
		{
			msg: "one invalid backend",
			backends: []graph.BackendRef{
				{
					Name:   "backend1",
					Valid:  false,
					Weight: 1,
				},
			},
			expSplit: false,
		},
		{
			msg: "multiple valid backends",
			backends: []graph.BackendRef{
				{
					Name:   "backend1",
					Valid:  true,
					Weight: 1,
				},
				{
					Name:   "backend2",
					Valid:  true,
					Weight: 1,
				},
			},
			expSplit: true,
		},
		{
			msg: "multiple backends - one invalid",
			backends: []graph.BackendRef{
				{
					Name:   "backend1",
					Valid:  true,
					Weight: 1,
				},
				{
					Name:   "backend2",
					Valid:  false,
					Weight: 1,
				},
			},
			expSplit: true,
		},
	}

	for _, test := range tests {
		bg := graph.BackendGroup{
			Source:   types.NamespacedName{Namespace: "test", Name: "hr"},
			Backends: test.backends,
		}
		result := backendGroupNeedsSplit(bg)
		if result != test.expSplit {
			t.Errorf("backendGroupNeedsSplit() mismatch for %q; expected %t", test.msg, result)
		}
	}
}

func TestBackendGroupName(t *testing.T) {
	tests := []struct {
		msg      string
		expName  string
		backends []graph.BackendRef
	}{
		{
			msg:      "empty backends",
			backends: []graph.BackendRef{},
			expName:  invalidBackendRef,
		},
		{
			msg:      "nil backends",
			backends: nil,
			expName:  invalidBackendRef,
		},
		{
			msg: "one valid backend with non-zero weight",
			backends: []graph.BackendRef{
				{
					Name:   "backend1",
					Valid:  true,
					Weight: 1,
				},
			},
			expName: "backend1",
		},
		{
			msg: "one valid backend with zero weight",
			backends: []graph.BackendRef{
				{
					Name:   "backend1",
					Valid:  true,
					Weight: 0,
				},
			},
			expName: invalidBackendRef,
		},
		{
			msg: "one invalid backend",
			backends: []graph.BackendRef{
				{
					Name:   "backend1",
					Valid:  false,
					Weight: 1,
				},
			},
			expName: invalidBackendRef,
		},
		{
			msg: "multiple valid backends",
			backends: []graph.BackendRef{
				{
					Name:   "backend1",
					Valid:  true,
					Weight: 1,
				},
				{
					Name:   "backend2",
					Valid:  true,
					Weight: 1,
				},
			},
			expName: "test__hr_rule0",
		},
		{
			msg: "multiple invalid backends",
			backends: []graph.BackendRef{
				{
					Name:   "backend1",
					Valid:  false,
					Weight: 1,
				},
				{
					Name:   "backend2",
					Valid:  false,
					Weight: 1,
				},
			},
			expName: "test__hr_rule0",
		},
	}

	for _, test := range tests {
		bg := graph.BackendGroup{
			Source:   types.NamespacedName{Namespace: "test", Name: "hr"},
			RuleIdx:  0,
			Backends: test.backends,
		}
		result := backendGroupName(bg)
		if result != test.expName {
			t.Errorf("backendGroupName() mismatch for %q; expected %s, got %s", test.msg, test.expName, result)
		}
	}
}
