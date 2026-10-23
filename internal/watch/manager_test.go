package watch

import (
	"context"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"

	automationv1alpha1 "github.com/eryalito/kubensync-operator/api/v1alpha1"
)

var configMaps = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}

func cmKey(name string) Key {
	return Key{GVR: configMaps, Namespace: "ns", Name: name}
}

// fakeWatches records the watches a Manager starts and lets tests fire their events.
type fakeWatches struct {
	t        *testing.T
	mu       sync.Mutex
	starts   map[Key]int
	contexts map[Key]context.Context
	notify   map[Key]func()
	// neverSynced makes every watch fail its initial list, as it does without permission to list.
	neverSynced bool
}

func (f *fakeWatches) start(ctx context.Context, key Key, onEvent func()) (func() bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.contexts[key]; ok && c.Err() == nil {
		f.t.Errorf("%s started while its watch is still running", key.Name)
	}
	f.starts[key]++
	f.contexts[key] = ctx
	f.notify[key] = onEvent
	return func() bool { return !f.neverSynced }, nil
}

func (f *fakeWatches) running() sets.Set[Key] {
	f.mu.Lock()
	defer f.mu.Unlock()
	running := sets.New[Key]()
	for key, c := range f.contexts {
		if c.Err() == nil {
			running.Insert(key)
		}
	}
	return running
}

func newTestManager(t *testing.T) (*Manager, *fakeWatches) {
	f := &fakeWatches{t: t, starts: map[Key]int{}, contexts: map[Key]context.Context{}, notify: map[Key]func(){}}
	m := NewManager(f.start, 200*time.Millisecond)
	go func() { _ = m.Start(t.Context()) }()
	return m, f
}

// routed fires an event for key, as its watch would, and returns the ManagedResources it reaches.
func routed(m *Manager, f *fakeWatches, key Key) []string {
	f.mu.Lock()
	notify := f.notify[key]
	f.mu.Unlock()
	if notify == nil {
		return nil
	}
	notify()
	var names []string
	for {
		select {
		case e := <-m.Events():
			names = append(names, e.Object)
		default:
			slices.Sort(names)
			return names
		}
	}
}

func mustSync(t *testing.T, m *Manager, name string, keys ...Key) {
	t.Helper()
	if err := m.Sync(t.Context(), name, keys); err != nil {
		t.Fatal(err)
	}
}

func expect(t *testing.T, m *Manager, f *fakeWatches, routes map[Key][]string, starts map[Key]int) {
	t.Helper()
	wantRunning := sets.New[Key]()
	for key, names := range routes {
		if len(names) > 0 {
			wantRunning.Insert(key)
		}
	}
	if got := f.running(); !got.Equal(wantRunning) {
		t.Errorf("running watches = %v, want %v", sets.List(keyNames(got)), sets.List(keyNames(wantRunning)))
	}
	for key, want := range routes {
		if got := routed(m, f, key); !slices.Equal(got, want) {
			t.Errorf("event on %s reached %v, want %v", key.Name, got, want)
		}
	}
	for key, want := range starts {
		if got := f.starts[key]; got != want {
			t.Errorf("%s started %d times, want %d", key.Name, got, want)
		}
	}
}

func keyNames(keys sets.Set[Key]) sets.Set[string] {
	names := sets.New[string]()
	for key := range keys {
		names.Insert(key.Name)
	}
	return names
}

func TestSameObjectReferencedTwiceByOneManagedResource(t *testing.T) {
	m, f := newTestManager(t)
	x := cmKey("x")

	mustSync(t, m, "mr1", x, x)
	expect(t, m, f, map[Key][]string{x: {"mr1"}}, map[Key]int{x: 1})

	// One of the two references stops watching: the other one keeps the watch.
	mustSync(t, m, "mr1", x)
	expect(t, m, f, map[Key][]string{x: {"mr1"}}, map[Key]int{x: 1})

	mustSync(t, m, "mr1")
	expect(t, m, f, map[Key][]string{x: nil}, map[Key]int{x: 1})

	mustSync(t, m, "mr1", x)
	expect(t, m, f, map[Key][]string{x: {"mr1"}}, map[Key]int{x: 2})
}

func TestSameObjectWatchedByTwoManagedResources(t *testing.T) {
	m, f := newTestManager(t)
	x := cmKey("x")

	mustSync(t, m, "mr1", x)
	mustSync(t, m, "mr2", x)
	expect(t, m, f, map[Key][]string{x: {"mr1", "mr2"}}, map[Key]int{x: 1})

	mustSync(t, m, "mr1")
	expect(t, m, f, map[Key][]string{x: {"mr2"}}, map[Key]int{x: 1})

	m.Remove("mr2")
	expect(t, m, f, map[Key][]string{x: nil}, map[Key]int{x: 1})
}

func TestRemoveKeepsObjectsStillWatchedByOthers(t *testing.T) {
	m, f := newTestManager(t)
	x, y := cmKey("x"), cmKey("y")

	mustSync(t, m, "mr1", x, y)
	mustSync(t, m, "mr2", y)
	m.Remove("mr1")
	expect(t, m, f, map[Key][]string{x: nil, y: {"mr2"}}, map[Key]int{x: 1, y: 1})
}

func TestReferenceMovedToAnotherObject(t *testing.T) {
	m, f := newTestManager(t)
	x, y := cmKey("x"), cmKey("y")

	mustSync(t, m, "mr1", x)
	mustSync(t, m, "mr1", y)
	expect(t, m, f, map[Key][]string{x: nil, y: {"mr1"}}, map[Key]int{x: 1, y: 1})
}

func TestSyncWithUnchangedKeysStartsNothing(t *testing.T) {
	m, f := newTestManager(t)
	x := cmKey("x")

	for range 3 {
		mustSync(t, m, "mr1", x)
	}
	expect(t, m, f, map[Key][]string{x: {"mr1"}}, map[Key]int{x: 1})
}

func TestRemoveIsIdempotent(t *testing.T) {
	m, f := newTestManager(t)
	x := cmKey("x")

	m.Remove("unknown")
	mustSync(t, m, "mr1", x)
	m.Remove("mr1")
	m.Remove("mr1")
	mustSync(t, m, "mr1")
	expect(t, m, f, map[Key][]string{x: nil}, map[Key]int{x: 1})
}

func TestWatchesThatNeverSyncStayRegistered(t *testing.T) {
	m, f := newTestManager(t)
	f.neverSynced = true
	x := cmKey("x")

	if err := m.Sync(t.Context(), "mr1", []Key{x}); err == nil {
		t.Error("expected an error for a watch that never completes its initial list")
	}
	// The watch keeps retrying, so its events still reach the ManagedResource once it works.
	expect(t, m, f, map[Key][]string{x: {"mr1"}}, map[Key]int{x: 1})
}

// TestRandomOperations compares the Manager with watchers recomputed from scratch after every operation.
func TestRandomOperations(t *testing.T) {
	m, f := newTestManager(t)
	objects := []Key{cmKey("a"), cmKey("b"), cmKey("c")}
	mrs := []string{"mr1", "mr2", "mr3"}
	type ref struct {
		object int
		watch  bool
	}
	specs := map[string][]ref{}
	wantStarts := map[Key]int{}
	watchers := func() map[Key][]string {
		w := map[Key][]string{}
		for _, key := range objects {
			w[key] = nil
		}
		for _, name := range mrs {
			seen := sets.New[Key]()
			for _, r := range specs[name] {
				if r.watch && !seen.Has(objects[r.object]) {
					seen.Insert(objects[r.object])
					w[objects[r.object]] = append(w[objects[r.object]], name)
				}
			}
		}
		for key := range w {
			slices.Sort(w[key])
		}
		return w
	}

	rng := rand.New(rand.NewPCG(1, 2))
	for i := range 2000 {
		before := watchers()
		name := mrs[rng.IntN(len(mrs))]
		spec := specs[name]
		switch op := rng.IntN(5); {
		case op == 0 || len(spec) == 0:
			spec = append(spec, ref{object: rng.IntN(len(objects)), watch: rng.IntN(2) == 0})
		case op == 1:
			j := rng.IntN(len(spec))
			spec[j].watch = !spec[j].watch
		case op == 2:
			spec[rng.IntN(len(spec))].object = rng.IntN(len(objects))
		case op == 3:
			j := rng.IntN(len(spec))
			spec = append(spec[:j], spec[j+1:]...)
		case op == 4 && rng.IntN(3) == 0:
			spec = nil
		}
		specs[name] = spec

		if len(spec) == 0 && rng.IntN(2) == 0 {
			m.Remove(name)
		} else {
			var keys []Key
			for _, r := range spec {
				if r.watch {
					keys = append(keys, objects[r.object])
				}
			}
			mustSync(t, m, name, keys...)
		}

		after := watchers()
		for _, key := range objects {
			if len(before[key]) == 0 && len(after[key]) > 0 {
				wantStarts[key]++
			}
		}
		expect(t, m, f, after, wantStarts)
		if t.Failed() {
			t.Fatalf("diverged at operation %d on %s, specs %v", i, name, specs)
		}
	}
}

func TestKeys(t *testing.T) {
	mapper := meta.NewDefaultRESTMapper(nil)
	mapper.Add(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"}, meta.RESTScopeRoot)
	ref := func(name, kind, group string) automationv1alpha1.ManagedResourceSpecTemplateDataRef {
		return automationv1alpha1.ManagedResourceSpecTemplateDataRef{Name: name, Namespace: "ns", Kind: kind, Group: group, ApiVersion: "v1"}
	}
	data := []automationv1alpha1.ManagedResourceSpecTemplateData{
		{Name: "cm", Type: automationv1alpha1.ConfigMap, Ref: ref("x", "", ""), Watch: true},
		{Name: "same-cm", Type: automationv1alpha1.KubernetesResource, Ref: ref("x", "ConfigMap", ""), Watch: true},
		{Name: "unwatched", Type: automationv1alpha1.ConfigMap, Ref: ref("y", "", "")},
		{Name: "secret", Type: automationv1alpha1.Secret, Ref: ref("s", "", ""), Watch: true},
		{Name: "cluster", Type: automationv1alpha1.KubernetesResource, Ref: ref("r", "ClusterRole", "rbac.authorization.k8s.io"), Watch: true},
		{Name: "unknown", Type: automationv1alpha1.KubernetesResource, Ref: ref("w", "Widget", "example.com"), Watch: true},
	}

	keys, err := Keys(data, mapper)
	if err == nil {
		t.Error("expected an error for the unknown kind")
	}
	want := []Key{
		cmKey("x"),
		cmKey("x"),
		{GVR: schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, Namespace: "ns", Name: "s"},
		{GVR: schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}, Name: "r"},
	}
	if !slices.Equal(keys, want) {
		t.Errorf("Keys() = %v, want %v", keys, want)
	}
}
