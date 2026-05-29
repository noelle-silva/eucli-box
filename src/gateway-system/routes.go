package gateway

func (s *system) registerRoutes() {
	s.mux.HandleFunc("POST /api/runs", s.handleStartRun)
	s.mux.HandleFunc("GET /api/runs/{runID}", s.handleGetRun)
	s.mux.HandleFunc("POST /api/runs/{runID}/cancel", s.handleCancelRun)
	s.mux.HandleFunc("POST /api/tool-confirmations", s.handleToolConfirmation)

	s.mux.HandleFunc("GET /api/roles", s.handleListRoles)
	s.mux.HandleFunc("POST /api/roles", s.handleSaveRole)
	s.mux.HandleFunc("GET /api/roles/{roleID}", s.handleLoadRole)
	s.mux.HandleFunc("DELETE /api/roles/{roleID}", s.handleDeleteRole)

	s.mux.HandleFunc("GET /api/providers", s.handleListProviders)
	s.mux.HandleFunc("POST /api/providers", s.handleSaveProvider)
	s.mux.HandleFunc("GET /api/providers/{providerID}", s.handleLoadProvider)
	s.mux.HandleFunc("DELETE /api/providers/{providerID}", s.handleDeleteProvider)
	s.mux.HandleFunc("POST /api/providers/{providerID}/models/refresh", s.handleRefreshProviderModels)

	s.mux.HandleFunc("GET /api/tools", s.handleListTools)
	s.mux.HandleFunc("POST /api/tools", s.handleSaveTool)
	s.mux.HandleFunc("GET /api/tools/{toolID}", s.handleLoadTool)

	s.mux.HandleFunc("GET /ws/events", s.handleEventsWebSocket)
}
