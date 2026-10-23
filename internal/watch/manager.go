package watch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/metadata/metadatainformer"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
)

var watchLogger = ctrl.Log.WithName("watch")

// Key identifies a watched object by its resource, so different ways of referencing the same object share a watch.
type Key struct {
	GVR       schema.GroupVersionResource
	Namespace string
	Name      string
}

// StartFunc starts watching key until ctx is cancelled, calling onEvent whenever the object changes. It returns
// without waiting for the initial list; synced reports when that list has completed.
type StartFunc func(ctx context.Context, key Key, onEvent func()) (synced func() bool, err error)

// Manager keeps one watch per object referenced by the ManagedResources and routes its events to them.
type Manager struct {
	start       StartFunc
	syncTimeout time.Duration
	events      chan event.TypedGenericEvent[string]

	started chan struct{}
	ctx     context.Context

	mu sync.Mutex
	// refs holds the objects each ManagedResource watches, and index the ManagedResources watching each object.
	// An object has a running watch exactly while its index entry is not empty.
	refs     map[string]sets.Set[Key]
	index    map[Key]sets.Set[string]
	watching map[Key]context.CancelFunc
}

func NewManager(start StartFunc, syncTimeout time.Duration) *Manager {
	return &Manager{
		start:       start,
		syncTimeout: syncTimeout,
		events:      make(chan event.TypedGenericEvent[string], 1024),
		started:     make(chan struct{}),
		refs:        map[string]sets.Set[Key]{},
		index:       map[Key]sets.Set[string]{},
		watching:    map[Key]context.CancelFunc{},
	}
}

// Events delivers the name of each ManagedResource that watches an object that changed.
func (m *Manager) Events() <-chan event.TypedGenericEvent[string] {
	return m.events
}

// Start keeps the context the watches run in, so they outlive the reconcile that starts them.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx = ctx
	close(m.started)
	<-ctx.Done()
	return nil
}

func (m *Manager) NeedLeaderElection() bool {
	return true
}

// Sync makes the ManagedResource name watch exactly keys, starting and stopping watches as needed. It waits up to
// syncTimeout for the initial list of new watches, so a change made after it returns is never missed. Calls are
// expected not to overlap (reconciles hold reconciler.Mutex): an overlapping call could return before a watch it
// shares has completed its initial list.
func (m *Manager) Sync(ctx context.Context, name string, keys []Key) error {
	select {
	case <-m.started:
	case <-ctx.Done():
		return ctx.Err()
	}
	newKeys := sets.New(keys...)

	m.mu.Lock()
	oldKeys := m.refs[name]
	for key := range oldKeys.Difference(newKeys) {
		m.release(name, key)
	}
	toStart := map[Key]context.Context{}
	for key := range newKeys.Difference(oldKeys) {
		if m.index[key] == nil {
			m.index[key] = sets.New[string]()
			watchCtx, cancel := context.WithCancel(m.ctx)
			m.watching[key] = cancel
			toStart[key] = watchCtx
		}
		m.index[key].Insert(name)
	}
	if newKeys.Len() == 0 {
		delete(m.refs, name)
	} else {
		m.refs[name] = newKeys
	}
	m.mu.Unlock()

	var errs []error
	var synced []cache.InformerSynced
	for key, watchCtx := range toStart {
		hasSynced, err := m.start(watchCtx, key, func() { m.notify(key) })
		if err != nil {
			errs = append(errs, fmt.Errorf("watching %s %s/%s: %w", key.GVR, key.Namespace, key.Name, err))
			continue
		}
		synced = append(synced, hasSynced)
	}
	if len(synced) > 0 {
		syncCtx, cancel := context.WithTimeout(ctx, m.syncTimeout)
		defer cancel()
		if !cache.WaitForCacheSync(syncCtx.Done(), synced...) {
			errs = append(errs, fmt.Errorf("initial list of %d new watches not completed within %s", len(synced), m.syncTimeout))
		}
	}
	return errors.Join(errs...)
}

// Remove stops the ManagedResource name from watching anything.
func (m *Manager) Remove(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.refs[name] {
		m.release(name, key)
	}
	delete(m.refs, name)
}

func (m *Manager) release(name string, key Key) {
	m.index[key].Delete(name)
	if m.index[key].Len() > 0 {
		return
	}
	m.watching[key]()
	delete(m.watching, key)
	delete(m.index, key)
}

func (m *Manager) notify(key Key) {
	m.mu.Lock()
	names := m.index[key].UnsortedList()
	m.mu.Unlock()
	for _, name := range names {
		select {
		case m.events <- event.TypedGenericEvent[string]{Object: name}:
		case <-m.ctx.Done():
			return
		}
	}
}

// Keys returns the objects of the watched data references. References whose kind cannot be resolved are
// skipped and reported in the error.
func Keys(data []automationv1alpha1.ManagedResourceSpecTemplateData, mapper meta.RESTMapper) ([]Key, error) {
	var keys []Key
	var errs []error
	for _, d := range data {
		if !d.Watch {
			continue
		}
		switch d.Type {
		case automationv1alpha1.ConfigMap:
			keys = append(keys, Key{GVR: schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}, Namespace: d.Ref.Namespace, Name: d.Ref.Name})
		case automationv1alpha1.Secret:
			keys = append(keys, Key{GVR: schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, Namespace: d.Ref.Namespace, Name: d.Ref.Name})
		case automationv1alpha1.KubernetesResource:
			mapping, err := mapper.RESTMapping(schema.GroupKind{Group: d.Ref.Group, Kind: d.Ref.Kind}, d.Ref.ApiVersion)
			if err != nil {
				errs = append(errs, fmt.Errorf("data %q: %w", d.Name, err))
				continue
			}
			namespace := d.Ref.Namespace
			if mapping.Scope.Name() == meta.RESTScopeNameRoot {
				namespace = ""
			}
			keys = append(keys, Key{GVR: mapping.Resource, Namespace: namespace, Name: d.Ref.Name})
		}
	}
	return keys, errors.Join(errs...)
}

// MetadataInformers starts a metadata-only informer per object, limited to it by a field selector.
func MetadataInformers(client metadata.Interface) StartFunc {
	return func(ctx context.Context, key Key, onEvent func()) (func() bool, error) {
		informer := metadatainformer.NewFilteredMetadataInformer(client, key.GVR, key.Namespace, 0, cache.Indexers{}, func(o *metav1.ListOptions) {
			o.FieldSelector = fields.OneTermEqualSelector("metadata.name", key.Name).String()
		}).Informer()
		if err := informer.SetWatchErrorHandlerWithContext(func(_ context.Context, _ *cache.Reflector, err error) {
			watchLogger.Error(err, "Error watching data resource", "resource", key.GVR.String(), "namespace", key.Namespace, "name", key.Name)
		}); err != nil {
			return nil, err
		}
		if _, err := informer.AddEventHandler(cache.ResourceEventHandlerDetailedFuncs{
			AddFunc: func(_ interface{}, isInInitialList bool) {
				if !isInInitialList {
					onEvent()
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				if resourceVersion(oldObj) != resourceVersion(newObj) {
					onEvent()
				}
			},
			DeleteFunc: func(interface{}) { onEvent() },
		}); err != nil {
			return nil, err
		}
		go informer.RunWithContext(ctx)
		return informer.HasSynced, nil
	}
}

func resourceVersion(obj interface{}) string {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return ""
	}
	return accessor.GetResourceVersion()
}
