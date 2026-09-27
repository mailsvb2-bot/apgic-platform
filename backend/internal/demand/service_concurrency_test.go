package demand

import (
	"errors"
	"sync"
	"testing"
)

func TestExclusiveHoldConcurrentClientsHaveSingleWinner(t *testing.T) {
	service := NewConformanceService(nil)
	first, err := service.CreateIntent("нужна помощь со сном")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateIntent("тоже не сплю")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(first.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmIntent(second.ID, []string{"sleep"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := service.Slots("spec-lebedeva")
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots=%#v err=%v", slots, err)
	}

	type outcome struct {
		hold *Hold
		err  error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, intent := range []*Intent{first, second} {
		intent := intent
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			hold, err := service.AcquireHold(intent.ID, slots[0].ID, intent.ClientIdentityID)
			outcomes <- outcome{hold: hold, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(outcomes)

	winners := 0
	conflicts := 0
	for result := range outcomes {
		switch {
		case result.err == nil && result.hold != nil && result.hold.State == "ACTIVE":
			winners++
		case errors.Is(result.err, ErrSlotHeld):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent outcome: hold=%#v err=%v", result.hold, result.err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("winners=%d conflicts=%d", winners, conflicts)
	}
}
