package safetyreminder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Weather 为发送当日天气摘要，来自中国天气网 d1 接口(国内直连、免密钥)。
type Weather struct {
	CityName    string
	Text        string // 预报天气现象，如"雷阵雨转大雨"
	MaxTempC    int
	MaxWindText string
	Alarms      []string // 官方气象预警信号标题
	FetchedAt   time.Time
}

// Kind 返回当天应插播的专题类别: rain=台风暴雨, heat=高温, 空串=常规轮换。
func (w *Weather) Kind() string {
	if w == nil {
		return ""
	}
	text := w.Text
	for _, a := range w.Alarms {
		text += " " + a
	}
	// 官方预警与天气现象共同判定，风暴类优先于高温
	stormKeys := []string{"台风", "暴雨", "大暴雨", "雷", "大雨"}
	for _, k := range stormKeys {
		if strings.Contains(text, k) {
			return "rain"
		}
	}
	if strings.Contains(text, "高温") {
		return "heat"
	}
	if strings.Contains(w.Text, "雨") {
		return "rain"
	}
	if w.MaxTempC >= 35 {
		return "heat"
	}
	return ""
}

func (w *Weather) Summary() string {
	if w == nil {
		return "无天气数据"
	}
	alarm := ""
	if len(w.Alarms) > 0 {
		alarm = " 预警:" + strings.Join(w.Alarms, "/")
	}
	return fmt.Sprintf("%s %s 最高%d℃%s", w.CityName, w.Text, w.MaxTempC, alarm)
}

type dzWeatherInfo struct {
	CityName string `json:"cityname"`
	Temp     string `json:"temp"`
	TempN    string `json:"tempn"`
	Weather  string `json:"weather"`
	WS       string `json:"ws"`
}

type dzAlarm struct {
	W []struct {
		Title string `json:"w5"`
	} `json:"w"`
}

var (
	weatherCache   Weather
	weatherCacheOk bool
	weatherMu      sync.Mutex
)

// FetchWeather 查询城市当日天气(10 分钟缓存)。cityCode 为中国天气网城市代码。
func FetchWeather(ctx context.Context, cityCode string) (*Weather, error) {
	weatherMu.Lock()
	defer weatherMu.Unlock()
	if weatherCacheOk && time.Since(weatherCache.FetchedAt) < 10*time.Minute {
		return &weatherCache, nil
	}
	if strings.TrimSpace(cityCode) == "" {
		cityCode = "101230501"
	}
	client := &http.Client{Timeout: 8 * time.Second}
	pageBody, err := fetchDZPage(ctx, client, cityCode)
	if err != nil {
		return nil, err
	}
	infoBody, ok := extractVarJSON(pageBody, "cityDZ"+cityCode)
	if !ok {
		return nil, fmt.Errorf("天气数据缺少 cityDZ 变量")
	}
	var wrapper struct {
		WeatherInfo dzWeatherInfo `json:"weatherinfo"`
	}
	if err := json.Unmarshal(infoBody, &wrapper); err != nil {
		return nil, fmt.Errorf("解析天气数据失败: %w", err)
	}
	info := wrapper.WeatherInfo
	weather := Weather{
		CityName:    info.CityName,
		Text:        info.Weather,
		MaxWindText: info.WS,
		FetchedAt:   time.Now(),
	}
	weather.MaxTempC, _ = strconv.Atoi(strings.TrimRight(strings.TrimSpace(info.Temp), "℃度"))
	if alarmBody, ok := extractVarJSON(pageBody, "alarmDZ"+cityCode); ok {
		var alarm dzAlarm
		if err := json.Unmarshal(alarmBody, &alarm); err == nil {
			for _, w := range alarm.W {
				if w.Title != "" {
					weather.Alarms = append(weather.Alarms, w.Title)
				}
			}
		}
	}
	weatherCache = weather
	weatherCacheOk = true
	return &weather, nil
}

func fetchDZPage(ctx context.Context, client *http.Client, cityCode string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://d1.weather.com.cn/dingzhi/"+cityCode+".html", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Referer", "http://www.weather.com.cn/")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("天气接口请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("天气接口状态码 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// extractVarJSON 从 "var name ={...};" 片段中按变量名截取完整 JSON(括号配平)。
func extractVarJSON(text, varName string) ([]byte, bool) {
	pos := strings.Index(text, varName)
	if pos < 0 {
		return nil, false
	}
	rel := strings.Index(text[pos:], "{")
	if rel < 0 {
		return nil, false
	}
	start := pos + rel
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return []byte(text[start : i+1]), true
			}
		}
	}
	return nil, false
}
