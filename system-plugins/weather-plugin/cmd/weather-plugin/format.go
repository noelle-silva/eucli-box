package main

import (
	"fmt"
	"strings"
	"time"
)

func formatDetail(bundle weatherBundle, config pluginConfig) string {
	parts := []string{
		fmt.Sprintf("【天气位置】%s%s", bundle.City.Name, locationSuffix(bundle.City)),
		formatCurrent(bundle.Now),
	}
	if config.IncludeAirQuality && bundle.Air != nil {
		parts = append(parts, formatAir(*bundle.Air))
	}
	if config.IncludeWarnings {
		parts = append(parts, formatWarnings(bundle.Warnings))
	}
	parts = append(parts, formatHourly(bundle.Hourly, config), formatDaily(bundle.Daily, config.ForecastDays))
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func formatBrief(bundle weatherBundle, config pluginConfig) string {
	items := []string{fmt.Sprintf("【当前天气】%s%s，%s，温度 %s℃", bundle.City.Name, locationSuffix(bundle.City), bundle.Now.Text, bundle.Now.Temp)}
	if bundle.Now.FeelsLike != "" {
		items = append(items, "体感 "+bundle.Now.FeelsLike+"℃")
	}
	if bundle.Now.WindDir != "" || bundle.Now.WindScale != "" {
		items = append(items, bundle.Now.WindDir+bundle.Now.WindScale+"级")
	}
	if config.IncludeAirQuality && bundle.Air != nil && bundle.Air.Category != "" {
		items = append(items, "空气质量 "+bundle.Air.Category+"("+bundle.Air.AQI+")")
	}
	if config.IncludeWarnings && len(bundle.Warnings) > 0 {
		items = append(items, "有天气预警："+bundle.Warnings[0].Title)
	}
	return strings.Join(nonEmptyStrings(items), "，")
}

func formatCurrent(now currentWeather) string {
	lines := []string{"【实时天气】"}
	lines = append(lines, "天气："+now.Text)
	lines = append(lines, "温度："+now.Temp+"℃")
	if now.FeelsLike != "" {
		lines = append(lines, "体感温度："+now.FeelsLike+"℃")
	}
	if now.WindDir != "" || now.WindScale != "" {
		lines = append(lines, "风况："+now.WindDir+now.WindScale+"级")
	}
	if now.Humidity != "" {
		lines = append(lines, "湿度："+now.Humidity+"%")
	}
	if now.Precip != "" {
		lines = append(lines, "降水量："+now.Precip+"毫米")
	}
	if now.Pressure != "" {
		lines = append(lines, "气压："+now.Pressure+"百帕")
	}
	if now.Vis != "" {
		lines = append(lines, "能见度："+now.Vis+"公里")
	}
	if now.ObsTime != "" {
		lines = append(lines, "观测时间："+formatWeatherTime(now.ObsTime))
	}
	return strings.Join(lines, "\n")
}

func formatAir(air airQuality) string {
	lines := []string{"【空气质量】"}
	if air.Category != "" || air.AQI != "" {
		lines = append(lines, "空气质量："+air.Category+" (AQI "+air.AQI+")")
	}
	if air.Primary != "" {
		lines = append(lines, "主要污染物："+air.Primary)
	}
	if air.PM25 != "" {
		lines = append(lines, "PM2.5："+air.PM25)
	}
	return strings.Join(lines, "\n")
}

func formatWarnings(warnings []weatherWarning) string {
	lines := []string{"【天气预警】"}
	if len(warnings) == 0 {
		return "【天气预警】\n当前无天气预警。"
	}
	for _, warning := range warnings {
		line := "- " + warning.Title
		if warning.SeverityColor != "" {
			line += "（" + warning.SeverityColor + "）"
		}
		if warning.TypeName != "" {
			line += "：" + warning.TypeName
		}
		lines = append(lines, line)
		if warning.PubTime != "" {
			lines = append(lines, "  发布时间："+formatWeatherTime(warning.PubTime))
		}
		if warning.Text != "" {
			lines = append(lines, "  内容："+warning.Text)
		}
	}
	return strings.Join(lines, "\n")
}

func formatHourly(hourly []hourlyWeather, config pluginConfig) string {
	lines := []string{"【未来小时预报】"}
	shown := 0
	for index := 0; index < len(hourly) && shown < config.HourlyForecastCount; index += config.HourlyForecastInterval {
		item := hourly[index]
		line := fmt.Sprintf("- %s：%s，%s℃", formatWeatherTime(item.FxTime), item.Text, item.Temp)
		if item.WindDir != "" || item.WindScale != "" {
			line += "，" + item.WindDir + item.WindScale + "级"
		}
		if item.Pop != "" {
			line += "，降水概率 " + item.Pop + "%"
		}
		lines = append(lines, line)
		shown++
	}
	return strings.Join(lines, "\n")
}

func formatDaily(daily []dailyWeather, days int) string {
	lines := []string{fmt.Sprintf("【未来%d日预报】", days)}
	for _, item := range daily {
		line := fmt.Sprintf("- %s：白天%s / 夜间%s，%s~%s℃", item.FxDate, item.TextDay, item.TextNight, item.TempMin, item.TempMax)
		if item.WindDirDay != "" || item.WindScaleDay != "" {
			line += "，白天" + item.WindDirDay + item.WindScaleDay + "级"
		}
		if item.Humidity != "" {
			line += "，湿度 " + item.Humidity + "%"
		}
		if item.UVIndex != "" {
			line += "，紫外线指数 " + item.UVIndex
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func locationSuffix(city cityInfo) string {
	parts := []string{}
	if city.Adm2 != "" && city.Adm2 != city.Name {
		parts = append(parts, city.Adm2)
	}
	if city.Adm1 != "" && city.Adm1 != city.Adm2 {
		parts = append(parts, city.Adm1)
	}
	if len(parts) == 0 {
		return ""
	}
	return "（" + strings.Join(parts, " / ") + "）"
}

func formatWeatherTime(value string) string {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.Format("2006-01-02 15:04")
	}
	return value
}

func nonEmptyStrings(values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
