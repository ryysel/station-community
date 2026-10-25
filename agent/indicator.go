package main

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

// Existing AirPlay integration creates led.suspend while Alice owns the ring.
// We only read these markers; we never pause AirPlay or modify its files.
func aliceOrAirplayOwnsRing() bool {
	for _, p := range []string{"/data/airplay/active", "/data/airplay/led.suspend"} {
		if _, err := os.Stat(p); err == nil || !os.IsNotExist(err) {
			return true
		}
	}
	return false
}

// ringIndicator is the light that shows Jarvis is awake and thinking. Station 2
// exposes it as a tiny framebuffer; Mini 2, Plum and Prune expose plain LED
// class devices. Both are driven without touching Quasar.
type ringIndicator interface {
	pulse()
	pulseFor(time.Duration, byte)
	think(context.Context) func()
	off()
	close()
}

func openIndicator() (ringIndicator, error) {
	p, fbErr := openWakePulse()
	if fbErr == nil {
		return p, nil
	}
	r, ledErr := openSysfsRing()
	if ledErr == nil {
		return r, nil
	}
	return nil, errors.New(fbErr.Error() + "; " + ledErr.Error())
}

type wakePulse struct {
	mu    sync.Mutex
	f     *os.File
	timer *time.Timer
	saved []byte
	frame []byte
}

func openWakePulse() (*wakePulse, error) {
	size, err := os.ReadFile("/sys/class/graphics/fb0/virtual_size")
	if err != nil || strings.TrimSpace(string(size)) != "89,1" {
		return nil, errors.New("not a Station 2 89-pixel framebuffer")
	}
	f, err := os.OpenFile("/dev/fb0", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	p := &wakePulse{f: f, frame: approvedWakeFrame()}
	return p, nil
}

func approvedWakeFrame() []byte {
	frame := make([]byte, 356)
	for i := 0; i < 89; i++ {
		// User visually confirmed this native channel appears blue on this
		// Station 2. Do not infer its physical colour from nominal BGRA order.
		frame[4*i+2] = 140
		frame[4*i+3] = 255
	}
	return frame
}

func isOurRingFrame(frame []byte) bool {
	if len(frame) != 356 || frame[2] == 0 || frame[2] > 140 {
		return false
	}
	br := frame[2]
	for i := 0; i < 89; i++ {
		if frame[4*i] != 0 || frame[4*i+1] != 0 || frame[4*i+2] != br || frame[4*i+3] != 255 {
			return false
		}
	}
	return true
}

func (p *wakePulse) restore() {
	if p.f == nil || p.saved == nil {
		return
	}
	current := make([]byte, 356)
	// If the vendor or AirPlay already changed our frame, leave it alone.
	if !aliceOrAirplayOwnsRing() {
		if _, err := p.f.ReadAt(current, 0); err == nil && bytes.Equal(current, p.frame) {
			// The idle ring is dark. Never restore a frame that Quasar
			// repainted while the dialog was being cancelled, otherwise the
			// light stays on after the answer.
			p.f.WriteAt(make([]byte, 356), 0)
		}
	}
	p.saved = nil
}

func (p *wakePulse) pulse() {
	p.pulseFor(1200*time.Millisecond, 140)
}

func (p *wakePulse) pulseFor(duration time.Duration, brightness byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.f == nil || aliceOrAirplayOwnsRing() {
		return
	}
	if p.timer != nil {
		p.timer.Stop()
	}
	p.restore()
	for i := 0; i < 89; i++ {
		p.frame[4*i+2] = brightness
	}
	p.saved = make([]byte, 356)
	if _, err := p.f.ReadAt(p.saved, 0); err != nil {
		p.saved = nil
		return
	}
	if _, err := p.f.WriteAt(p.frame, 0); err != nil {
		p.saved = nil
		return
	}
	p.timer = time.AfterFunc(duration, func() { p.mu.Lock(); defer p.mu.Unlock(); p.restore() })
}

func (p *wakePulse) off() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	p.restore()
	if p.f != nil && !aliceOrAirplayOwnsRing() {
		current := make([]byte, 356)
		if _, err := p.f.ReadAt(current, 0); err == nil && isOurRingFrame(current) {
			p.f.WriteAt(make([]byte, 356), 0)
		}
	}
}

func (p *wakePulse) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.timer != nil {
		p.timer.Stop()
	}
	p.restore()
	if p.f != nil {
		p.f.Close()
		p.f = nil
	}
}

// Keep the user-approved hardware channel, breathing while the cloud turn is
// pending. Stop writing immediately when Alice/AirPlay owns the ring.
func (p *wakePulse) think(parent context.Context) func() {
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
				p.mu.Lock()
				if p.f == nil || aliceOrAirplayOwnsRing() {
					p.mu.Unlock()
					continue
				}
				if p.timer != nil {
					p.timer.Stop()
					p.timer = nil
				}
				// Take the frame over instead of giving up when Quasar repaints
				// the ring after the dialog is cancelled.
				if p.saved == nil {
					p.saved = make([]byte, 356)
					if _, err := p.f.ReadAt(p.saved, 0); err != nil {
						p.saved = nil
						p.mu.Unlock()
						return
					}
				}
				br := byte(65 + 70*(.5+.5*math.Sin(time.Since(start).Seconds()*2*math.Pi)))
				for i := 0; i < 89; i++ {
					p.frame[4*i+2] = br
				}
				_, err := p.f.WriteAt(p.frame, 0)
				p.mu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { cancel(); <-done; p.off() }) }
}
