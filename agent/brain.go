package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type cloudConfig struct {
	Provider        string   `json:"provider"`
	APIKey          string   `json:"api_key"`
	BaseURL         string   `json:"base_url"`
	ChatModel       string   `json:"chat_model"`
	ChatFallback    string   `json:"chat_fallback"`
	SystemPrompt    string   `json:"system_prompt"`
	TimeOffsetHours int      `json:"time_offset_hours"`
	DailyTurnLimit  int      `json:"daily_turn_limit"`
	TTSProvider     string   `json:"tts_provider"`
	WakeWords       []string `json:"wake_words"`
	FollowUpSeconds int      `json:"follow_up_seconds"`
	City            string   `json:"city"`
	Latitude        float64  `json:"latitude"`
	Longitude       float64  `json:"longitude"`
	WeatherEnabled  bool     `json:"weather_enabled"`
	CloudWaitMS     int      `json:"cloud_wait_ms"`
	StationID       string   `json:"station_id"`
	StationPriority int      `json:"station_priority"`
	Peers           []string `json:"peers"`
	ArbPort         int      `json:"arb_port"`
}

type brain struct {
	cfg     cloudConfig
	http    *http.Client
	weather *weatherCache
}

func loadBrain(path string, logf func(string, ...any)) (*brain, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg cloudConfig
	if err = json.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if cfg.Provider == "" {
		cfg.Provider = "gemini"
	}
	if cfg.ChatModel == "" {
		return nil, errors.New("chat_model is required")
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("STATION_AGENT_API_KEY")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("api_key is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.FollowUpSeconds <= 0 {
		cfg.FollowUpSeconds = 5
	}
	var weather *weatherCache
	if cfg.WeatherEnabled {
		if strings.TrimSpace(cfg.City) == "" || cfg.Latitude < -90 || cfg.Latitude > 90 || cfg.Longitude < -180 || cfg.Longitude > 180 {
			return nil, errors.New("weather requires a city and valid coordinates")
		}
		weather = newWeatherCache(cfg.Latitude, cfg.Longitude, cfg.City, logf)
	}
	if cfg.CloudWaitMS <= 0 {
		cfg.CloudWaitMS = 1200
	}
	return &brain{
		cfg:     cfg,
		http:    &http.Client{Timeout: 20 * time.Second},
		weather: weather,
	}, nil
}

func (b *brain) system() string {
	clock := time.Now().In(time.FixedZone("station", b.cfg.TimeOffsetHours*3600))
	s := b.cfg.SystemPrompt
	s += " Сейчас с тобой разговаривают голосом: ответ будет озвучен без экрана."
	s += " Не используй списки, ссылки, markdown и латиницу без необходимости."
	if b.cfg.City != "" {
		s += " Ты находишься в городе " + b.cfg.City + "."
	}
	s += " Текущие местные дата и время: " + clock.Format("2006-01-02 15:04") + "."
	s += " Время и дату бери только из этой строки, это достоверные данные."
	if b.weather != nil {
		if w := b.weather.current(); w != "" {
			s += " Погода (достоверные данные): " + w
			s += " Вопросы про погоду на улице относи к этим данным."
		}
	}
	s += " Погоду не придумывай: если строки с погодой нет, скажи, что данные недоступны."
	s += " Про новости, курсы валют и прочие меняющиеся данные честно говори, что не знаешь."
	s += " Если реплика не адресована тебе, ответь ровно SKIP."
	return s
}

type modelPart struct {
	Text    string `json:"text"`
	Thought bool   `json:"thought"`
}

type modelResponse struct {
	Candidates []struct {
		Content struct {
			Parts []modelPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (b *brain) ask(ctx context.Context, question string) (string, error) {
	if b.cfg.Provider == "openai" {
		return b.askOpenAI(ctx, question)
	}
	payload := map[string]any{
		"contents":          []any{map[string]any{"role": "user", "parts": []any{map[string]string{"text": question}}}},
		"systemInstruction": map[string]any{"parts": []any{map[string]string{"text": b.system()}}},
		"generationConfig": map[string]any{
			"temperature":     0.3,
			"maxOutputTokens": 400,
			"thinkingConfig":  map[string]int{"thinkingBudget": 0},
		},
	}
	text, err := b.gemini(ctx, b.cfg.ChatModel, payload)
	if err != nil && b.cfg.ChatFallback != "" {
		text, err = b.gemini(ctx, b.cfg.ChatFallback, payload)
	}
	return text, err
}

func (b *brain) gemini(ctx context.Context, model string, payload any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	endpoint := "https://generativelanguage.googleapis.com/v1beta/models/" + model + ":generateContent"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", b.cfg.APIKey)
	res, err := b.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode != 200 {
		return "", fmt.Errorf("provider HTTP %d", res.StatusCode)
	}
	var parsed modelResponse
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Candidates) == 0 {
		return "", errors.New("empty response")
	}
	var out strings.Builder
	for _, p := range parsed.Candidates[0].Content.Parts {
		if !p.Thought {
			out.WriteString(p.Text)
		}
	}
	return cleanAnswer(out.String()), nil
}

func (b *brain) askOpenAI(ctx context.Context, question string) (string, error) {
	payload := map[string]any{
		"model": b.cfg.ChatModel,
		"messages": []any{
			map[string]string{"role": "system", "content": b.system()},
			map[string]string{"role": "user", "content": question},
		},
		"temperature": 0.3,
		"max_tokens":  400,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(b.cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.cfg.APIKey)
	res, err := b.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode != 200 {
		return "", fmt.Errorf("provider HTTP %d", res.StatusCode)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err = json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("empty response")
	}
	return cleanAnswer(parsed.Choices[0].Message.Content), nil
}

func cleanAnswer(s string) string {
	s = strings.TrimSpace(s)
	if strings.EqualFold(strings.Trim(s, " .!\"'«»"), "SKIP") {
		return ""
	}
	return s
}
