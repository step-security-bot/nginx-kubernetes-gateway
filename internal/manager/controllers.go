package manager

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctlr "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/nginxinc/nginx-kubernetes-gateway/internal/reconciler"
)

const (
	// addIndexFieldTimeout is the timeout used for adding an Index Field to a cache.
	addIndexFieldTimeout = 2 * time.Minute
)

type controllerConfig struct {
	objectType           client.Object
	namespacedNameFilter reconciler.NamespacedNameFilterFunc // optional
	k8sEventFilter       predicate.Predicate                 // optional
	fieldIndexes         map[string]client.IndexerFunc       // optional
}

// newReconciler creates a new Implementation. Used for unit testing.
var newReconciler = reconciler.NewImplementation

func registerController(
	ctx context.Context,
	mgr manager.Manager,
	eventCh chan interface{},
	cfg controllerConfig,
) error {
	for field, indexerFunc := range cfg.fieldIndexes {
		err := addIndex(ctx, mgr.GetFieldIndexer(), cfg.objectType, field, indexerFunc)
		if err != nil {
			return fmt.Errorf("failed to add index when registering a controller for %T: %w", cfg.objectType, err)
		}
	}

	builder := ctlr.NewControllerManagedBy(mgr).For(cfg.objectType)

	if cfg.k8sEventFilter != nil {
		builder = builder.WithEventFilter(cfg.k8sEventFilter)
	}

	recCfg := reconciler.Config{
		Getter:               mgr.GetClient(),
		ObjectType:           cfg.objectType,
		EventCh:              eventCh,
		NamespacedNameFilter: cfg.namespacedNameFilter,
	}

	err := builder.Complete(newReconciler(recCfg))
	if err != nil {
		return fmt.Errorf("cannot build a controller for %T: %w", cfg.objectType, err)
	}

	return nil
}

func addIndex(ctx context.Context, indexer client.FieldIndexer, objectType client.Object, field string, indexerFunc client.IndexerFunc) error {
	c, cancel := context.WithTimeout(ctx, addIndexFieldTimeout)
	defer cancel()

	err := indexer.IndexField(c, objectType, field, indexerFunc)
	if err != nil {
		return fmt.Errorf("failed to add index for %T for field %s: %w", objectType, field, err)
	}

	return nil
}

func createFilterForGatewayClass(gcName string) reconciler.NamespacedNameFilterFunc {
	return func(nsname types.NamespacedName) (bool, string) {
		if nsname.Name != gcName {
			return false, fmt.Sprintf("GatewayClass is ignored because this controller only supports the GatewayClass %s", gcName)
		}
		return true, ""
	}
}
