package main

import (
	"context"
	"encoding/json"
	"os"
	"time"
)

func main() {
	decoder := json.NewDecoder(os.Stdin)
	decoder.UseNumber()
	var req pluginRequest
	if err := decoder.Decode(&req); err != nil {
		writeResponse(pluginResponse{Status: "failed", Error: "输入不是有效 JSON"})
		return
	}
	if req.Action != actionResolvePlaceholders {
		writeResponse(pluginResponse{Status: "failed", Error: "不支持的插件动作"})
		return
	}
	config, err := loadConfig(req.DefaultConfig, req.UserConfig)
	if err != nil {
		writeResponse(pluginResponse{Status: "failed", Error: err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	bundle, err := newWeatherClient().fetchWeather(ctx, config)
	if err != nil {
		writeResponse(pluginResponse{Status: "failed", Error: err.Error()})
		return
	}
	writeResponse(pluginResponse{Status: "success", Values: map[string]string{weatherDetailInterfaceID: formatDetail(bundle, config), weatherBriefInterfaceID: formatBrief(bundle, config)}})
}

func writeResponse(resp pluginResponse) {
	_ = json.NewEncoder(os.Stdout).Encode(resp)
}
