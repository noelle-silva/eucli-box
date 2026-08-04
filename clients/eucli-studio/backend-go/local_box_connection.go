package main

import (
	"errors"
	"strings"
)

const (
	boxConnectionSourceLocal  = "local"
	boxConnectionSourceManual = "manual"
)

type boxConnection struct {
	Source     string
	BaseURL    string
	Credential string
}

func (connection *boxConnection) valid() bool {
	return connection != nil &&
		(connection.Source == boxConnectionSourceLocal || connection.Source == boxConnectionSourceManual) &&
		strings.TrimSpace(connection.BaseURL) != "" &&
		strings.TrimSpace(connection.Credential) != ""
}

func (s *service) currentBoxConnection() (*boxConnection, error) {
	s.connectionMu.RLock()
	connection := s.businessConnection
	s.connectionMu.RUnlock()
	if connection != nil && connection.valid() {
		copy := *connection
		return &copy, nil
	}
	if s.localBox != nil {
		if connection := s.localBox.currentConnection(); connection != nil && connection.valid() {
			return connection, nil
		}
	}
	cfg, err := s.config.load()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.EucliBoxURL) == "" {
		return nil, errors.New("EUCLI_BOX_NOT_CONFIGURED")
	}
	return &boxConnection{Source: boxConnectionSourceManual, BaseURL: cfg.EucliBoxURL, Credential: cfg.EucliBoxKey}, nil
}
