package hub

import (
	"testing"
	"time"
)

func TestPublishFansOutAndUnsubscribeStops(t *testing.T) {
	h := New()

	first, unsubscribeFirst := h.Subscribe("topic")
	defer unsubscribeFirst()
	second, unsubscribeSecond := h.Subscribe("topic")
	defer unsubscribeSecond()

	h.Publish("topic", Event{Name: "one", Data: []byte("a")})

	for name, ch := range map[string]<-chan Event{"first": first, "second": second} {
		select {
		case event := <-ch:
			if event.Name != "one" || string(event.Data) != "a" {
				t.Errorf("%s event = %+v", name, event)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s subscriber did not receive the event", name)
		}
	}

	unsubscribeFirst()
	h.Publish("topic", Event{Name: "two"})

	select {
	case event := <-first:
		t.Errorf("unsubscribed channel received %+v", event)
	case <-time.After(10 * time.Millisecond):
	}

	select {
	case event := <-second:
		if event.Name != "two" {
			t.Errorf("second event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("remaining subscriber did not receive the event")
	}
}

func TestPublishDoesNotBlockOnFullSubscriber(t *testing.T) {
	h := New()
	ch, unsubscribe := h.Subscribe("topic")
	defer unsubscribe()

	for i := 0; i < 100; i++ {
		h.Publish("topic", Event{Name: "event"})
	}

	if len(ch) != cap(ch) {
		t.Errorf("buffered events = %d, want the buffer full at %d", len(ch), cap(ch))
	}
}

func TestTopicsAreDistinct(t *testing.T) {
	if UserTopic(1) == ThreadTopic(1) {
		t.Errorf("user and thread topics collide: %q", UserTopic(1))
	}
	if UserTopic(1) == UserTopic(2) {
		t.Errorf("user topics collide: %q", UserTopic(1))
	}
}
