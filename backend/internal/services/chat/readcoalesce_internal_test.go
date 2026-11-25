package chat

import (
	"testing"

	"ovc/backend/internal/domain"
)

func TestReadWindowsSignalOnceAtOnceAndOnceMoreOnlyWhenDirty(t *testing.T) {
	windows := newReadWindows()
	key := readKey{reader: domain.UserID{1}, channel: domain.ChannelID{2}}

	first, generation := windows.exposed(key)
	second, sameGeneration := windows.exposed(key)
	third, _ := windows.exposed(key)
	trailing := windows.ended(key, generation)
	clean := windows.ended(key, generation)
	afterRemoval := windows.ended(key, generation)
	reopened, nextGeneration := windows.exposed(key)

	if first != readRunNow || second != readIgnore || third != readIgnore || sameGeneration != generation {
		t.Fatalf("a burst decided %v, %v and %v in generations %d and %d", first, second, third, generation, sameGeneration)
	}
	if trailing != readRunTrailing || clean != readRemove || afterRemoval != readIgnore {
		t.Fatalf("the window ends decided %v, %v and %v", trailing, clean, afterRemoval)
	}
	if reopened != readRunNow || nextGeneration == generation {
		t.Fatalf("the next exposure decided %v in generation %d", reopened, nextGeneration)
	}
}

func TestReadWindowsIgnoreAStaleTimerAndEverythingAfterClose(t *testing.T) {
	windows := newReadWindows()
	key := readKey{reader: domain.UserID{1}, channel: domain.ChannelID{2}}
	other := readKey{reader: domain.UserID{3}, channel: domain.ChannelID{2}}
	_, stale := windows.exposed(key)
	windows.ended(key, stale)
	_, current := windows.exposed(key)
	_, otherGeneration := windows.exposed(other)
	windows.exposed(other)

	staleEnd := windows.ended(key, stale)
	windows.close()
	afterClose := []readDecision{windows.ended(key, current), windows.ended(other, otherGeneration)}
	exposedAfterClose, closedGeneration := windows.exposed(key)

	if staleEnd != readIgnore {
		t.Fatalf("a stale timer decided %v", staleEnd)
	}
	for _, decision := range afterClose {
		if decision != readIgnore {
			t.Fatalf("a window end after close decided %v", decision)
		}
	}
	if exposedAfterClose != readIgnore || closedGeneration != 0 || len(windows.open) != 0 {
		t.Fatalf("an exposure after close decided %v in generation %d with %d open windows", exposedAfterClose, closedGeneration, len(windows.open))
	}
}
