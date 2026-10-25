package main

import "testing"

func TestBeatsOrdersStations(t *testing.T) {
	cases := []struct {
		name string
		a, b arbClaim
		want bool
	}{
		{"higher priority wins", arbClaim{Station: "s2", Priority: 20}, arbClaim{Station: "s1", Priority: 10}, true},
		{"lower priority loses", arbClaim{Station: "s1", Priority: 10}, arbClaim{Station: "s2", Priority: 20}, false},
		{"tie broken by id", arbClaim{Station: "station-a"}, arbClaim{Station: "station-d"}, true},
		{"tie same id", arbClaim{Station: "station-d"}, arbClaim{Station: "station-d"}, false},
	}
	for _, c := range cases {
		if got := beats(c.a, c.b); got != c.want {
			t.Errorf("%s: beats(%+v, %+v) = %v, want %v", c.name, c.a, c.b, got, c.want)
		}
	}
}

// The election must never elect two stations: for any pair exactly one of the
// two orders is true.
func TestBeatsIsAntisymmetric(t *testing.T) {
	stations := []arbClaim{
		{Station: "station-a", Priority: 10},
		{Station: "station-b", Priority: 30},
		{Station: "station-c", Priority: 20},
		{Station: "station-d", Priority: 40},
		{Station: "station-aaa", Priority: 30},
	}
	for _, a := range stations {
		for _, b := range stations {
			if a.Station == b.Station {
				continue
			}
			if beats(a, b) == beats(b, a) {
				t.Errorf("ambiguous election between %+v and %+v", a, b)
			}
		}
	}
}

func TestWinsWithoutPeersIsLocal(t *testing.T) {
	a := &arbiter{self: "station-d", priority: 40}
	if !a.wins(0) {
		t.Fatal("a lone station must always be elected")
	}
}
