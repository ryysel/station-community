package main

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

// sysfsRing drives the blue ring on the models that expose it as plain LED
// class devices (Station Mini 2, Plum, Prune) instead of a framebuffer.
// Nothing in Quasar is modified: we only set brightness while Jarvis is awake
// and switch the ring off when the turn is over.
//
// The idle state of the ring is "off", so finishing always writes 0. Saving and
// restoring the previous value is deliberately not done: Quasar repaints the
// ring while a dialog is cancelled, and restoring such a value leaves the ring
// glowing long after the answer.
type sysfsRing struct {
	mu    sync.Mutex
	paths []string
	timer *time.Timer
}

func openSysfsRing() (*sysfsRing, error) {
	paths, err := filepath.Glob("/sys/class/leds/*:blue:ring*/brightness")
	if err != nil || len(paths) == 0 {
		return nil, errors.New("no blue ring LEDs")
	}
	sort.Strings(paths)
	return &sysfsRing{paths: paths}, nil
}

func (r *sysfsRing) setAll(level byte) {
	text := []byte(strconv.Itoa(int(level)))
	for _, p := range r.paths {
		if err := os.WriteFile(p, text, 0644); err != nil {
			return
		}
	}
}

func (r *sysfsRing) pulse() {
	r.pulseFor(1200*time.Millisecond, 140)
}

func (r *sysfsRing) pulseFor(d time.Duration, brightness byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if aliceOrAirplayOwnsRing() {
		return
	}
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	r.setAll(brightness)
	r.timer = time.AfterFunc(d, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.timer = nil
		r.setAll(0)
	})
}

func (r *sysfsRing) off() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	r.setAll(0)
}

func (r *sysfsRing) close() { r.off() }

// think keeps the ring breathing while the cloud turn is pending and turns it
// off when the answer is ready. It takes over the pending pulse timer so the
// two never fight over the ring.
func (r *sysfsRing) think(parent context.Context) func() {
	r.mu.Lock()
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	r.mu.Unlock()

	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		start := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if aliceOrAirplayOwnsRing() {
					continue
				}
				br := byte(65 + 70*(.5+.5*math.Sin(time.Since(start).Seconds()*2*math.Pi)))
				r.mu.Lock()
				r.setAll(br)
				r.mu.Unlock()
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { cancel(); <-done; r.off() }) }
}
