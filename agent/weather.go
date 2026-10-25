package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const weatherTTL = 10 * time.Minute

// weatherCache keeps a short description of the weather around the station.
// Refreshes happen in the background, so a voice turn never waits for the
// network: the model simply gets the last known description.
type weatherCache struct {
	client *http.Client
	lat    float64
	lon    float64
	place  string
	logf   func(string, ...any)

	mu       sync.Mutex
	text     string
	fetched  time.Time
	inflight bool
	coldWait bool
}

func newWeatherCache(lat, lon float64, place string, logf func(string, ...any)) *weatherCache {
	return &weatherCache{
		client: &http.Client{Timeout: 4 * time.Second},
		lat:    lat,
		lon:    lon,
		place:  place,
		logf:   logf,
	}
}

// current returns the cached description and schedules a refresh when the
// cache is empty or older than weatherTTL.
func (w *weatherCache) current() string {
	w.mu.Lock()
	text, fetched := w.text, w.fetched
	if (text == "" || time.Since(fetched) > weatherTTL) && !w.inflight {
		w.inflight = true
		go w.refresh()
	}
	w.mu.Unlock()
	if text != "" {
		return text
	}
	// Cold start: wait a moment for the background fetch instead of telling
	// the user that the weather is unavailable. Only once, so an offline
	// station never slows down every turn.
	w.mu.Lock()
	wait := !w.coldWait
	w.coldWait = true
	w.mu.Unlock()
	if !wait {
		return ""
	}
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		w.mu.Lock()
		text = w.text
		w.mu.Unlock()
		if text != "" {
			break
		}
	}
	return text
}

func (w *weatherCache) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	text, err := w.fetch(ctx)
	w.mu.Lock()
	w.inflight = false
	if err == nil && text != "" {
		w.text, w.fetched = text, time.Now()
	}
	w.mu.Unlock()
	if err != nil {
		w.logf("weather: %v", err)
	}
}

type openMeteo struct {
	Current struct {
		Temperature float64 `json:"temperature_2m"`
		Apparent    float64 `json:"apparent_temperature"`
		Code        int     `json:"weather_code"`
		Wind        float64 `json:"wind_speed_10m"`
	} `json:"current"`
	Daily struct {
		Max    []float64 `json:"temperature_2m_max"`
		Min    []float64 `json:"temperature_2m_min"`
		Precip []float64 `json:"precipitation_probability_max"`
	} `json:"daily"`
}

func (w *weatherCache) fetch(ctx context.Context) (string, error) {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", w.lat))
	q.Set("longitude", fmt.Sprintf("%.4f", w.lon))
	q.Set("current", "temperature_2m,apparent_temperature,weather_code,wind_speed_10m")
	q.Set("daily", "temperature_2m_max,temperature_2m_min,precipitation_probability_max")
	q.Set("timezone", "Europe/Moscow")
	q.Set("forecast_days", "2")
	endpoint := "https://api.open-meteo.com/v1/forecast?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", err
	}
	res, err := w.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", fmt.Errorf("open-meteo HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 256<<10))
	if err != nil {
		return "", err
	}
	var parsed openMeteo
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	return describeWeather(w.place, parsed), nil
}

func describeWeather(place string, r openMeteo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Сейчас в %s %s, %s, ощущается как %s",
		place, degrees(r.Current.Temperature), weatherRu(r.Current.Code), degrees(r.Current.Apparent))
	if r.Current.Wind >= 1 {
		fmt.Fprintf(&b, ", ветер %.0f %s в секунду", r.Current.Wind, metersWord(r.Current.Wind))
	}
	if len(r.Daily.Min) > 0 && len(r.Daily.Max) > 0 {
		fmt.Fprintf(&b, ". Сегодня от %s до %s", degrees(r.Daily.Min[0]), degrees(r.Daily.Max[0]))
		if len(r.Daily.Precip) > 0 {
			fmt.Fprintf(&b, ", вероятность осадков %.0f процентов", r.Daily.Precip[0])
		}
	}
	if len(r.Daily.Min) > 1 && len(r.Daily.Max) > 1 {
		fmt.Fprintf(&b, ". Завтра от %s до %s", degrees(r.Daily.Min[1]), degrees(r.Daily.Max[1]))
		if len(r.Daily.Precip) > 1 {
			fmt.Fprintf(&b, ", вероятность осадков %.0f процентов", r.Daily.Precip[1])
		}
	}
	b.WriteString(".")
	return b.String()
}

// degrees renders a temperature the way it is spoken: "-3" is read as
// "минус три", so no unit suffix is needed and no grammar rule is broken.
func degrees(v float64) string {
	n := math.Round(v)
	if n < 0 {
		return fmt.Sprintf("минус %.0f", -n)
	}
	return fmt.Sprintf("%.0f", n)
}

func metersWord(v float64) string {
	n := int(math.Abs(math.Round(v)))
	if n%10 == 1 && n%100 != 11 {
		return "метр"
	}
	if n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14) {
		return "метра"
	}
	return "метров"
}

// weatherRu maps WMO weather codes to short Russian descriptions.
func weatherRu(code int) string {
	switch code {
	case 0:
		return "ясно"
	case 1:
		return "почти ясно"
	case 2:
		return "переменная облачность"
	case 3:
		return "пасмурно"
	case 45, 48:
		return "туман"
	case 51, 53, 55:
		return "морось"
	case 56, 57:
		return "ледяная морось"
	case 61:
		return "небольшой дождь"
	case 63:
		return "дождь"
	case 65:
		return "сильный дождь"
	case 66, 67:
		return "ледяной дождь"
	case 71:
		return "небольшой снег"
	case 73:
		return "снег"
	case 75:
		return "сильный снег"
	case 77:
		return "снежная крупа"
	case 80, 81, 82:
		return "ливень"
	case 85, 86:
		return "снегопад"
	case 95:
		return "гроза"
	case 96, 99:
		return "гроза с градом"
	}
	return "погода без описания"
}
