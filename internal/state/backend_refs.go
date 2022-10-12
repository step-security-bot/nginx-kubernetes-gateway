package state

import (
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/state/resolver"
)

type (
	// ruleIndex is the index of the HTTPRouteRule.
	ruleIndex int
	// resolvedBackends is a map of backend names to their endpoints.
	resolvedBackends map[string][]resolver.Endpoint
	// backendGroupsByRule is a map of rule indexes to backend groups.
	backendGroupsByRule map[ruleIndex]BackendGroup
)

// BackendRefs includes the BackendRefs of an HTTPRoute.
type BackendRefs struct {
	Errors   []string
	Resolved resolvedBackends
	ByRule   backendGroupsByRule
}

func newBackendRefs() BackendRefs {
	return BackendRefs{
		Errors:   make([]string, 0),
		Resolved: make(resolvedBackends),
		ByRule:   make(backendGroupsByRule),
	}
}

func resolveBackendRefs(
	ctx context.Context,
	routes map[types.NamespacedName]*route,
	services map[types.NamespacedName]*v1.Service,
	resolver resolver.ServiceResolver,
) Warnings {
	resolveBackendRefsForRoutes(ctx, routes, services, resolver)

	warnings := newWarnings()
	for _, r := range routes {
		for _, msg := range r.BackendRefs.Errors {
			warnings.AddWarningf(
				r.Source,
				"cannot resolve backend ref: %s",
				msg,
			)
		}
	}

	return warnings
}

func resolveBackendRefsForRoutes(
	ctx context.Context,
	routes map[types.NamespacedName]*route,
	services map[types.NamespacedName]*v1.Service,
	resolver resolver.ServiceResolver,
) {
	for _, r := range routes {
		for idx, rule := range r.Source.Spec.Rules {

			backends := make([]BackendRef, 0, len(rule.BackendRefs))

			for _, ref := range rule.BackendRefs {

				weight := int32(1)
				if ref.Weight != nil {
					weight = *ref.Weight
				}

				svc, port, err := getServiceAndPortFromRef(ref.BackendRef, r.Source.Namespace, services)
				if err != nil {
					backends = append(backends, BackendRef{Weight: weight})

					r.BackendRefs.Errors = append(r.BackendRefs.Errors, err.Error())

					continue
				}

				backendName := fmt.Sprintf("%s_%s_%d", svc.Namespace, svc.Name, port)

				backends = append(backends, BackendRef{
					Name:   backendName,
					Valid:  true,
					Weight: weight,
				})

				eps, err := resolver.Resolve(ctx, svc, int32(*ref.Port))
				if err != nil {
					r.BackendRefs.Errors = append(r.BackendRefs.Errors, err.Error())
				}

				// We still add the endpoints to the resolved map even if there was an error.
				// This is because we want to generate an upstream for every valid Service,
				// even if it doesn't have endpoints.
				r.BackendRefs.Resolved[backendName] = eps
			}

			r.BackendRefs.ByRule[ruleIndex(idx)] = BackendGroup{
				Source:   client.ObjectKeyFromObject(r.Source),
				RuleIdx:  idx,
				Backends: backends,
			}
		}
	}
}

func getServiceAndPortFromRef(
	ref v1beta1.BackendRef,
	routeNamespace string,
	services map[types.NamespacedName]*v1.Service,
) (*v1.Service, int32, error) {
	err := validateBackendRef(ref, routeNamespace)
	if err != nil {
		return nil, 0, err
	}

	svcNsName := types.NamespacedName{Name: string(ref.Name), Namespace: routeNamespace}

	svc, ok := services[svcNsName]
	if !ok {
		return nil, 0, fmt.Errorf("the Service %s does not exist", svcNsName)
	}

	// safe to dereference port here because we already validated that the port is not nil.
	return svc, int32(*ref.Port), nil
}

func validateBackendRef(ref v1beta1.BackendRef, routeNs string) error {
	if ref.Kind != nil && *ref.Kind != "Service" {
		return fmt.Errorf("the Kind must be Service; got %s", *ref.Kind)
	}

	if ref.Namespace != nil && string(*ref.Namespace) != routeNs {
		return fmt.Errorf("cross-namespace routing is not permitted; namespace %s does not match the HTTPRoute namespace %s", *ref.Namespace, routeNs)
	}

	if ref.Port == nil {
		return errors.New("port is missing")
	}

	return nil
}
