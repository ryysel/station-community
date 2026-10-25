package main

import (
	"strings"
	"testing"
)

func TestDescribeWeather(t *testing.T) {
	var r openMeteo
	r.Current.Temperature = -3.4
	r.Current.Apparent = -7.2
	r.Current.Code = 3
	r.Current.Wind = 4.6
	r.Daily.Min = []float64{-5.1, -6.4}
	r.Daily.Max = []float64{0.2, -1.0}
	r.Daily.Precip = []float64{20, 60}

	got := describeWeather("тестовом городе", r)
	for _, want := range []string{"тестовом городе", "минус 3", "пасмурно", "минус 7", "ветер 5 метров", "от минус 5 до 0", "20 процентов", "Завтра", "60 процентов"} {
		if !strings.Contains(got, want) {
			t.Fatalf("description %q does not contain %q", got, want)
		}
	}
}

func TestWeatherRu(t *testing.T) {
	if weatherRu(0) != "ясно" || weatherRu(95) != "гроза" {
		t.Fatalf("unexpected mapping: %q %q", weatherRu(0), weatherRu(95))
	}
}
