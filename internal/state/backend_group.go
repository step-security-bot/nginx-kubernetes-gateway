package state

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
)

// BackendGroup represents a group of backends for a rule in an HTTPRoute.
type BackendGroup struct {
	Source   types.NamespacedName
	RuleIdx  int
	Backends []BackendRef
}

// BackendRef is an internal representation of a backendRef in an HTTPRoute.
type BackendRef struct {
	Name   string
	Valid  bool
	Weight int32
}

// NeedsSplit returns true if traffic needs to be split among the backends in the group.
func (bg *BackendGroup) NeedsSplit() bool {
	return len(bg.Backends) > 1
}

// Name returns the name of the backend group.
// If the group needs to be split, the name returned is the name of the group.
// If the group doesn't need to be split, the name returned is the name of the backend if it is valid.
// If the name cannot be determined, it returns an empty string.
func (bg *BackendGroup) Name() string {
	switch len(bg.Backends) {
	case 0:
		return ""
	case 1:
		b := bg.Backends[0]
		if b.Weight <= 0 || !b.Valid {
			return ""
		}
		return b.Name
	default:
		return bg.GroupName()
	}
}

// GroupName returns the name of the backend group.
func (bg *BackendGroup) GroupName() string {
	return fmt.Sprintf("%s_%s_rule%d", bg.Source.Namespace, bg.Source.Name, bg.RuleIdx)
}
