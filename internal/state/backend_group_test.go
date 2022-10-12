package state_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/types"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/state"
)

func TestBackendGroup_NeedsSplit(t *testing.T) {
	tests := []struct {
		msg      string
		backends []state.BackendRef
		expSplit bool
	}{
		{
			msg:      "empty backends",
			backends: []state.BackendRef{},
			expSplit: false,
		},
		{
			msg:      "nil backends",
			backends: nil,
			expSplit: false,
		},
		{
			msg: "one valid backend",
			backends: []state.BackendRef{
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
			backends: []state.BackendRef{
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
			backends: []state.BackendRef{
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
			backends: []state.BackendRef{
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
		bg := state.BackendGroup{
			Source:   types.NamespacedName{Namespace: "test", Name: "hr"},
			Backends: test.backends,
		}
		result := bg.NeedsSplit()
		if result != test.expSplit {
			t.Errorf("BackendGroup.NeedsSplit() mismatch for %q; expected %t", test.msg, result)
		}
	}
}

func TestBackendGroup_Name(t *testing.T) {
	tests := []struct {
		msg      string
		backends []state.BackendRef
		expName  string
	}{
		{
			msg:      "empty backends",
			backends: []state.BackendRef{},
			expName:  "",
		},
		{
			msg:      "nil backends",
			backends: nil,
			expName:  "",
		},
		{
			msg: "one valid backend with non-zero weight",
			backends: []state.BackendRef{
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
			backends: []state.BackendRef{
				{
					Name:   "backend1",
					Valid:  true,
					Weight: 0,
				},
			},
			expName: "",
		},
		{
			msg: "one invalid backend",
			backends: []state.BackendRef{
				{
					Name:   "backend1",
					Valid:  false,
					Weight: 1,
				},
			},
			expName: "",
		},
		{
			msg: "multiple valid backends",
			backends: []state.BackendRef{
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
			expName: "test_hr_rule0",
		},
		{
			msg: "multiple invalid backends",
			backends: []state.BackendRef{
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
			expName: "test_hr_rule0",
		},
	}

	for _, test := range tests {
		bg := state.BackendGroup{
			Source:   types.NamespacedName{Namespace: "test", Name: "hr"},
			RuleIdx:  0,
			Backends: test.backends,
		}
		result := bg.Name()
		if result != test.expName {
			t.Errorf("BackendGroup.Name() mismatch for %q; expected %s, got %s", test.msg, test.expName, result)
		}
	}
}

func TestBackendGroup_GroupName(t *testing.T) {
	bg := state.BackendGroup{
		Source:  types.NamespacedName{Namespace: "test", Name: "hr"},
		RuleIdx: 20,
	}
	expected := "test_hr_rule20"
	result := bg.GroupName()
	if result != expected {
		t.Errorf("BackendGroup.GroupName() mismatch; expected %s, got %s", expected, result)
	}
}
