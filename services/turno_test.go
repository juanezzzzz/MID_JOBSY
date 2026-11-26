package services

import "testing"

func TestTurnoDe(t *testing.T) {
	for h, want := range map[string]int{"07:00": 1, "11:59": 1, "12:00": 2, "17:30": 2, "18:00": 3, "22:00": 3} {
		if got := turnoDe(h); got != want {
			t.Errorf("turnoDe(%s) = %d, quiero %d", h, got, want)
		}
	}
}
