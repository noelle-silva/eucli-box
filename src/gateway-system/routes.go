package gateway

func (s *system) registerRoutes() {
	s.mux.HandleFunc("POST /api/runs", s.authWrap(s.handleStartRun))
	s.mux.HandleFunc("GET /api/runs/{runID}", s.authWrap(s.handleGetRun))
	s.mux.HandleFunc("POST /api/runs/{runID}/cancel", s.authWrap(s.handleCancelRun))
	s.mux.HandleFunc("POST /api/tool-confirmations", s.authWrap(s.handleToolConfirmation))

	s.mux.HandleFunc("GET /api/roles", s.authWrap(s.handleListRoles))
	s.mux.HandleFunc("POST /api/roles", s.authWrap(s.handleSaveRole))
	s.mux.HandleFunc("GET /api/roles/{roleID}", s.authWrap(s.handleLoadRole))
	s.mux.HandleFunc("DELETE /api/roles/{roleID}", s.authWrap(s.handleDeleteRole))

	s.mux.HandleFunc("GET /api/roles/{roleID}/sessions", s.authWrap(s.handleListSessions))
	s.mux.HandleFunc("POST /api/roles/{roleID}/sessions", s.authWrap(s.handleSaveSession))
	s.mux.HandleFunc("GET /api/roles/{roleID}/sessions/{sessionID}", s.authWrap(s.handleLoadSession))
	s.mux.HandleFunc("DELETE /api/roles/{roleID}/sessions/{sessionID}", s.authWrap(s.handleDeleteSession))

	s.mux.HandleFunc("GET /api/providers", s.authWrap(s.handleListProviders))
	s.mux.HandleFunc("POST /api/providers", s.authWrap(s.handleSaveProvider))
	s.mux.HandleFunc("GET /api/providers/{providerID}", s.authWrap(s.handleLoadProvider))
	s.mux.HandleFunc("DELETE /api/providers/{providerID}", s.authWrap(s.handleDeleteProvider))
	s.mux.HandleFunc("POST /api/providers/{providerID}/models/refresh", s.authWrap(s.handleRefreshProviderModels))

	s.mux.HandleFunc("GET /api/tools", s.authWrap(s.handleListTools))
	s.mux.HandleFunc("POST /api/tools", s.authWrap(s.handleSaveTool))
	s.mux.HandleFunc("GET /api/tools/{toolID}", s.authWrap(s.handleLoadTool))

	s.mux.HandleFunc("GET /ws/events", s.handleEventsWebSocket)
}
