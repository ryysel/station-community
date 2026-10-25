package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type dailyBudget struct {
	Day           string `json:"day"`
	Used          int    `json:"used"`
	path          string
	limit, offset int
}

func openBudget(path string, limit, offset int) (*dailyBudget, error) {
	if limit <= 0 {
		limit = 80
	}
	b := &dailyBudget{path: path, limit: limit, offset: offset}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err = json.Unmarshal(data, b); err != nil {
			return nil, fmt.Errorf("invalid usage state: %w", err)
		}
		if _, err = time.Parse("2006-01-02", b.Day); err != nil || b.Used < 0 {
			return nil, fmt.Errorf("invalid usage date/count")
		}
	}
	return b, nil
}

func (b *dailyBudget) take(now time.Time) error {
	today := now.In(time.FixedZone("station", b.offset*3600)).Format("2006-01-02")
	if b.Day > today {
		return fmt.Errorf("clock moved behind usage date; cloud blocked")
	}
	used := b.Used
	if b.Day != today {
		used = 0
	}
	if used >= b.limit {
		return fmt.Errorf("daily cloud-turn limit reached (%d)", b.limit)
	}
	state := struct {
		Day  string `json:"day"`
		Used int    `json:"used"`
	}{today, used + 1}
	data, _ := json.Marshal(state)
	f, err := os.OpenFile(b.path+".new", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(b.path+".new", b.path); err != nil {
		return err
	}
	b.Day = today
	b.Used = used + 1
	return nil
}
