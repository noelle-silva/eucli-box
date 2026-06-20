# 天气信息插件

天气信息插件通过和风天气获取天气资料，并向系统插件子系统提供两个占位符接口。

## 提供的占位符接口

- `weather-detail`：默认占位符名为 `weather_detail`，输出详细天气报告。
- `weather-brief`：默认占位符名为 `weather_Brief`，输出当前天气简报。

## 配置项

- `city`：要查询天气的城市，例如 `北京`。
- `apiKey`：和风天气 API Key。
- `apiHost`：和风天气 API 域名，默认 `devapi.qweather.com`。
- `forecastDays`：多日预报天数，支持 `3`、`7`、`10`、`15`。
- `hourlyForecastInterval`：小时预报展示间隔，默认每 2 小时展示一条。
- `hourlyForecastCount`：小时预报展示条数，默认展示 12 条。
- `includeWarnings`：是否包含天气预警，默认开启。
- `includeAirQuality`：是否包含空气质量，默认关闭。

## 使用方式

1. 打开“设置 → 系统插件管理”，选择“天气信息插件”。
2. 填写城市、和风天气密钥和其他配置。
3. 打开“设置 → 占位符管理”，从插件接口创建 `weather_detail` 或 `weather_Brief`。
4. 在提示词中使用 `{{weather_detail}}` 或 `{{weather_Brief}}`。

## 行为边界

- 本插件是按需型插件，只在提示词解析需要天气值时运行。
- 必填配置缺失或天气服务请求失败时，插件会明确返回失败，对应占位符保持原样。
- 本插件第一版不包含月相和太阳角度。
