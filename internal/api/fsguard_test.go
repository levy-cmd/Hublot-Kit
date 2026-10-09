package api

import (
	"context"
	"errors"
	"testing"
	"time"
)

func busy(g *fsGuard) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.busy)
}

func TestGuardKeepsTheSlotUntilRelease(t *testing.T) {
	g := newFSGuard(1, time.Second)
	v, release, err := guarded(g, context.Background(), "c", func() (int, error) { return 7, nil }, nil)
	if v != 7 || err != nil {
		t.Fatalf("%d %v", v, err)
	}
	// The caller still uses the result (a download streams it): slot taken.
	if _, _, err := guarded(g, context.Background(), "c", func() (int, error) { return 1, nil }, nil); !errors.Is(err, errBusy) {
		t.Fatalf("attendu errBusy tant que release n'est pas appelé, reçu %v", err)
	}
	release()
	release() // idempotent
	if busy(g) != 0 {
		t.Fatal("créneau non rendu")
	}
}

func TestGuardFreesTheSlotOnError(t *testing.T) {
	g := newFSGuard(1, time.Second)
	_, release, err := guarded(g, context.Background(), "c", func() (int, error) { return 0, errors.New("x") }, nil)
	release() // a no-op, but always callable
	if err == nil || busy(g) != 0 {
		t.Fatalf("err %v, créneaux %d", err, busy(g))
	}
}

func TestGuardTimesOutAndCleansUpLateResult(t *testing.T) {
	g := newFSGuard(2, 20*time.Millisecond)
	unblock := make(chan struct{})
	cleaned := make(chan int, 1)
	_, release, err := guarded(g, context.Background(), "c", func() (int, error) { <-unblock; return 9, nil },
		func(v int) { cleaned <- v })
	release()
	if !errors.Is(err, errTimeout) {
		t.Fatalf("attendu errTimeout, reçu %v", err)
	}
	if busy(g) != 1 {
		t.Fatal("la goroutine bloquée doit garder son créneau")
	}
	close(unblock)
	select {
	case v := <-cleaned:
		if v != 9 {
			t.Fatalf("nettoyage de %d", v)
		}
	case <-time.After(time.Second):
		t.Fatal("le résultat tardif n'a pas été nettoyé")
	}
	deadline := time.Now().Add(time.Second)
	for busy(g) != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if busy(g) != 0 {
		t.Fatal("créneau non rendu après le vrai retour")
	}
}

func TestGuardBoundsEachContainer(t *testing.T) {
	g := newFSGuard(1, 20*time.Millisecond)
	block := make(chan struct{})
	defer close(block)
	if _, _, err := guarded(g, context.Background(), "c", func() (int, error) { <-block; return 0, nil }, nil); !errors.Is(err, errTimeout) {
		t.Fatalf("attendu errTimeout, reçu %v", err)
	}
	if _, _, err := guarded(g, context.Background(), "c", func() (int, error) { return 1, nil }, nil); !errors.Is(err, errBusy) {
		t.Fatalf("attendu errBusy, reçu %v", err)
	}
	v, release, err := guarded(g, context.Background(), "other", func() (int, error) { return 1, nil }, nil)
	release()
	if err != nil || v != 1 {
		t.Fatalf("un autre conteneur a son propre budget : %d %v", v, err)
	}
}

func TestGuardStopsWithTheRequest(t *testing.T) {
	g := newFSGuard(1, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	block := make(chan struct{})
	defer close(block)
	if _, _, err := guarded(g, ctx, "c", func() (int, error) { <-block; return 0, nil }, nil); !errors.Is(err, errTimeout) {
		t.Fatalf("requête annulée : %v", err)
	}
}
