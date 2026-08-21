package main

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"

	"eucli-box/pkg/toolcontrol"
)

const (
	controlAddressEnv  = "EUCLI_TOOL_CONTROL_ADDR"
	controlTokenEnv    = "EUCLI_TOOL_CONTROL_TOKEN"
	controlVersionEnv  = "EUCLI_TOOL_CONTROL_VERSION"
	controlRequiredEnv = "EUCLI_TOOL_CONTROL_REQUIRED"
)

func connectToolControl(ctx context.Context) (*toolcontrol.Client, error) {
	address := strings.TrimSpace(os.Getenv(controlAddressEnv))
	token := strings.TrimSpace(os.Getenv(controlTokenEnv))
	version := strings.TrimSpace(os.Getenv(controlVersionEnv))
	required := controlRequired()

	if !required && address == "" && token == "" && version == "" {
		return nil, nil
	}
	if address == "" || token == "" || version == "" {
		return nil, errors.New("tool control environment is incomplete")
	}
	parsedVersion, err := strconv.Atoi(version)
	if err != nil || parsedVersion != toolcontrol.ProtocolVersion {
		return nil, errors.New("tool control protocol version is invalid")
	}
	client, err := toolcontrol.Connect(ctx, address, token)
	if err != nil {
		return nil, err
	}
	if err := client.WaitReady(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func controlRequired() bool {
	return strings.TrimSpace(os.Getenv(controlRequiredEnv)) == "1"
}
