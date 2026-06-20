package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const requestTimeout = 12 * time.Second

type weatherClient struct {
	client *http.Client
}

type weatherBundle struct {
	City     cityInfo
	Now      currentWeather
	Hourly   []hourlyWeather
	Daily    []dailyWeather
	Warnings []weatherWarning
	Air      *airQuality
}

type cityInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Adm1      string `json:"adm1"`
	Adm2      string `json:"adm2"`
	Country   string `json:"country"`
	Lat       string `json:"lat"`
	Lon       string `json:"lon"`
	UTCOffset string `json:"utcOffset"`
}

type currentWeather struct {
	ObsTime   string `json:"obsTime"`
	Temp      string `json:"temp"`
	FeelsLike string `json:"feelsLike"`
	Text      string `json:"text"`
	WindDir   string `json:"windDir"`
	WindScale string `json:"windScale"`
	Humidity  string `json:"humidity"`
	Precip    string `json:"precip"`
	Pressure  string `json:"pressure"`
	Vis       string `json:"vis"`
}

type hourlyWeather struct {
	FxTime    string `json:"fxTime"`
	Temp      string `json:"temp"`
	Text      string `json:"text"`
	WindDir   string `json:"windDir"`
	WindScale string `json:"windScale"`
	Humidity  string `json:"humidity"`
	Pop       string `json:"pop"`
	Precip    string `json:"precip"`
}

type dailyWeather struct {
	FxDate         string `json:"fxDate"`
	TextDay        string `json:"textDay"`
	TextNight      string `json:"textNight"`
	TempMax        string `json:"tempMax"`
	TempMin        string `json:"tempMin"`
	WindDirDay     string `json:"windDirDay"`
	WindScaleDay   string `json:"windScaleDay"`
	WindDirNight   string `json:"windDirNight"`
	WindScaleNight string `json:"windScaleNight"`
	Humidity       string `json:"humidity"`
	Precip         string `json:"precip"`
	UVIndex        string `json:"uvIndex"`
}

type weatherWarning struct {
	Title         string `json:"title"`
	PubTime       string `json:"pubTime"`
	SeverityColor string `json:"severityColor"`
	TypeName      string `json:"typeName"`
	Text          string `json:"text"`
}

type airQuality struct {
	AQI      string
	Category string
	Primary  string
	PM25     string
}

type apiTextValue string

func (v *apiTextValue) UnmarshalJSON(raw []byte) error {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		*v = ""
		return nil
	}
	var stringValue string
	if err := json.Unmarshal(raw, &stringValue); err == nil {
		*v = apiTextValue(stringValue)
		return nil
	}
	var numberValue json.Number
	if err := json.Unmarshal(raw, &numberValue); err == nil {
		*v = apiTextValue(numberValue.String())
		return nil
	}
	var boolValue bool
	if err := json.Unmarshal(raw, &boolValue); err == nil {
		*v = apiTextValue(strconv.FormatBool(boolValue))
		return nil
	}
	return fmt.Errorf("无法读取文本值 %s", text)
}

func (v apiTextValue) String() string {
	return string(v)
}

func newWeatherClient() weatherClient {
	return weatherClient{client: &http.Client{Timeout: requestTimeout}}
}

func (c weatherClient) fetchWeather(ctx context.Context, config pluginConfig) (weatherBundle, error) {
	city, err := c.lookupCity(ctx, config)
	if err != nil {
		return weatherBundle{}, err
	}
	now, err := c.fetchCurrentWeather(ctx, config, city.ID)
	if err != nil {
		return weatherBundle{}, err
	}
	hourly, err := c.fetchHourlyWeather(ctx, config, city.ID)
	if err != nil {
		return weatherBundle{}, err
	}
	daily, err := c.fetchDailyWeather(ctx, config, city.ID)
	if err != nil {
		return weatherBundle{}, err
	}
	bundle := weatherBundle{City: city, Now: now, Hourly: hourly, Daily: daily}
	if config.IncludeWarnings {
		warnings, err := c.fetchWarnings(ctx, config, city.ID)
		if err != nil {
			return weatherBundle{}, err
		}
		bundle.Warnings = warnings
	}
	if config.IncludeAirQuality {
		air, err := c.fetchAirQuality(ctx, config, city.Lat, city.Lon)
		if err != nil {
			return weatherBundle{}, err
		}
		bundle.Air = &air
	}
	return bundle, nil
}

func (c weatherClient) lookupCity(ctx context.Context, config pluginConfig) (cityInfo, error) {
	var payload struct {
		Code     string     `json:"code"`
		Location []cityInfo `json:"location"`
	}
	endpoint := c.endpoint(config, "/geo/v2/city/lookup", map[string]string{"location": config.City})
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return cityInfo{}, err
	}
	if payload.Code != "200" || len(payload.Location) == 0 || strings.TrimSpace(payload.Location[0].ID) == "" {
		return cityInfo{}, fmt.Errorf("没有找到城市 %q 的天气位置", config.City)
	}
	return payload.Location[0], nil
}

func (c weatherClient) fetchCurrentWeather(ctx context.Context, config pluginConfig, cityID string) (currentWeather, error) {
	var payload struct {
		Code string         `json:"code"`
		Now  currentWeather `json:"now"`
	}
	endpoint := c.endpoint(config, "/v7/weather/now", map[string]string{"location": cityID})
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return currentWeather{}, err
	}
	if payload.Code != "200" || strings.TrimSpace(payload.Now.Text) == "" {
		return currentWeather{}, fmt.Errorf("当前天气获取失败")
	}
	return payload.Now, nil
}

func (c weatherClient) fetchHourlyWeather(ctx context.Context, config pluginConfig, cityID string) ([]hourlyWeather, error) {
	var payload struct {
		Code   string          `json:"code"`
		Hourly []hourlyWeather `json:"hourly"`
	}
	endpoint := c.endpoint(config, "/v7/weather/24h", map[string]string{"location": cityID})
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	if payload.Code != "200" || len(payload.Hourly) == 0 {
		return nil, fmt.Errorf("小时预报获取失败")
	}
	return payload.Hourly, nil
}

func (c weatherClient) fetchDailyWeather(ctx context.Context, config pluginConfig, cityID string) ([]dailyWeather, error) {
	var payload struct {
		Code  string         `json:"code"`
		Daily []dailyWeather `json:"daily"`
	}
	endpoint := c.endpoint(config, "/v7/weather/"+dailyEndpoint(config.ForecastDays), map[string]string{"location": cityID})
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	if payload.Code != "200" || len(payload.Daily) == 0 {
		return nil, fmt.Errorf("多日预报获取失败")
	}
	return payload.Daily, nil
}

func (c weatherClient) fetchWarnings(ctx context.Context, config pluginConfig, cityID string) ([]weatherWarning, error) {
	var payload struct {
		Code    string           `json:"code"`
		Warning []weatherWarning `json:"warning"`
	}
	endpoint := c.endpoint(config, "/v7/warning/now", map[string]string{"location": cityID})
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	if payload.Code != "200" {
		return nil, fmt.Errorf("天气预警获取失败")
	}
	return payload.Warning, nil
}

func (c weatherClient) fetchAirQuality(ctx context.Context, config pluginConfig, lat string, lon string) (airQuality, error) {
	var payload struct {
		Indexes []struct {
			Code             string       `json:"code"`
			AQI              apiTextValue `json:"aqi"`
			Category         string       `json:"category"`
			PrimaryPollutant struct {
				Name string `json:"name"`
			} `json:"primaryPollutant"`
		} `json:"indexes"`
		Pollutants []struct {
			Code          string `json:"code"`
			Concentration struct {
				Value apiTextValue `json:"value"`
			} `json:"concentration"`
		} `json:"pollutants"`
	}
	endpoint := c.endpoint(config, "/airquality/v1/current/"+url.PathEscape(lat)+"/"+url.PathEscape(lon), nil)
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return airQuality{}, err
	}
	if len(payload.Indexes) == 0 {
		return airQuality{}, fmt.Errorf("空气质量获取失败")
	}
	mainIndex := payload.Indexes[0]
	for _, item := range payload.Indexes {
		if item.Code == "us-epa" {
			mainIndex = item
			break
		}
	}
	air := airQuality{AQI: mainIndex.AQI.String(), Category: mainIndex.Category, Primary: mainIndex.PrimaryPollutant.Name}
	if air.Primary == "" {
		air.Primary = "无"
	}
	for _, item := range payload.Pollutants {
		if item.Code == "pm2p5" {
			air.PM25 = item.Concentration.Value.String()
			break
		}
	}
	return air, nil
}

func (c weatherClient) getJSON(ctx context.Context, endpoint string, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("天气服务请求失败：HTTP %d", response.StatusCode)
	}
	decoder := json.NewDecoder(response.Body)
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("天气服务返回内容无法读取：%w", err)
	}
	return nil
}

func (c weatherClient) endpoint(config pluginConfig, path string, params map[string]string) string {
	values := url.Values{}
	values.Set("key", config.APIKey)
	for key, value := range params {
		values.Set(key, value)
	}
	return "https://" + config.APIHost + path + "?" + values.Encode()
}

func dailyEndpoint(days int) string {
	switch days {
	case 3:
		return "3d"
	case 7:
		return "7d"
	case 10:
		return "10d"
	case 15:
		return "15d"
	default:
		return "3d"
	}
}
