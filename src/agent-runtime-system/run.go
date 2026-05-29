package agentruntime

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

func (s *system) StartRun(ctx context.Context, request types.RunRequest) (types.RunState, error) {
	if err := validateRunRequest(ctx, request); err != nil {
		return types.RunState{}, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	now := nowUTC()
	state := types.RunState{ID: utils.NewID("run"), RoleID: request.RoleID, SessionID: request.SessionID, Status: types.RunStatusCreated, CreatedAt: now, UpdatedAt: now}
	record := &runRecord{runID: state.ID, roleID: request.RoleID, state: state, cancel: cancel}
	s.mu.Lock()
	s.runs[state.ID] = record
	s.mu.Unlock()
	go s.run(runCtx, record, request)
	return state, nil
}

func (s *system) GetRun(ctx context.Context, runID string) (types.RunState, error) {
	if strings.TrimSpace(runID) == "" {
		return types.RunState{}, runtimeInvalid("run id is required", nil)
	}
	state, ok := s.getRunState(runID)
	if !ok {
		return types.RunState{}, runtimeNotFound("run was not found", nil)
	}
	return state, nil
}

func (s *system) CancelRun(ctx context.Context, runID string) error {
	s.mu.Lock()
	record, ok := s.runs[runID]
	if !ok {
		s.mu.Unlock()
		return runtimeNotFound("run was not found", nil)
	}
	record.cancel()
	s.mu.Unlock()
	state, err := s.updateRun(runID, types.RunStatusCancelled, "cancelled by user")
	if err != nil {
		return err
	}
	s.publish(runID, "run_cancelled", state)
	return nil
}

func (s *system) run(ctx context.Context, record *runRecord, request types.RunRequest) {
	state, err := s.updateRun(record.runID, types.RunStatusRunning, "")
	if err != nil {
		return
	}
	s.publish(record.runID, "run_started", state)
	session, err := s.loadOrCreateSession(ctx, request)
	if err != nil {
		s.failRun(context.Background(), record, session, err.Error())
		return
	}
	if record.state.SessionID == "" {
		if err := s.setRunSessionID(record.runID, session.ID); err != nil {
			s.failRun(context.Background(), record, session, err.Error())
			return
		}
	}
	session = appendMessage(session, userMessage(request.Message))
	if err := s.saveSession(ctx, session, types.RunStatusRunning); err != nil {
		s.failRun(context.Background(), record, session, err.Error())
		return
	}
	record.session = session
	for step := 1; step <= s.config.MaxSteps; step++ {
		if err := ctx.Err(); err != nil {
			s.cancelRunRecord(context.Background(), record, record.session)
			return
		}
		roleContext, err := s.buildRoleContext(ctx, record.roleID, record.session)
		if err != nil {
			s.failRun(context.Background(), record, record.session, err.Error())
			return
		}
		modelResponse, err := s.callModel(ctx, roleContext)
		if err != nil {
			s.failRun(context.Background(), record, record.session, err.Error())
			return
		}
		if err := ctx.Err(); err != nil {
			s.cancelRunRecord(context.Background(), record, record.session)
			return
		}
		record.session = appendMessage(record.session, assistantMessage(modelResponse.Content))
		s.publish(record.runID, "model_output", modelResponse)
		if err := s.saveSession(ctx, record.session, types.RunStatusRunning); err != nil {
			s.failRun(context.Background(), record, record.session, err.Error())
			return
		}
		if len(modelResponse.ToolIntents) == 0 {
			s.completeRun(context.Background(), record, record.session)
			return
		}
		if len(modelResponse.ToolIntents) > 1 {
			s.failRun(context.Background(), record, record.session, "model returned more than one tool intent")
			return
		}
		result, err := s.handleToolIntent(ctx, record, modelResponse.ToolIntents[0])
		if err != nil {
			s.failRun(context.Background(), record, record.session, err.Error())
			return
		}
		if err := ctx.Err(); err != nil {
			s.cancelRunRecord(context.Background(), record, record.session)
			return
		}
		s.publish(record.runID, "tool_result", result)
		if err := s.saveSession(ctx, record.session, types.RunStatusRunning); err != nil {
			s.failRun(context.Background(), record, record.session, err.Error())
			return
		}
	}
	s.failRun(context.Background(), record, record.session, "run exceeded max steps")
}

func validateRunRequest(ctx context.Context, request types.RunRequest) error {
	if err := ctx.Err(); err != nil {
		return runtimeInvalid("start run context is cancelled", err)
	}
	if strings.TrimSpace(request.RoleID) == "" {
		return runtimeInvalid("role id is required", nil)
	}
	if strings.TrimSpace(request.Message) == "" {
		return runtimeInvalid("message is required", nil)
	}
	return nil
}

func (s *system) completeRun(ctx context.Context, record *runRecord, session types.Session) {
	state, err := s.updateRun(record.runID, types.RunStatusCompleted, "")
	if err != nil {
		return
	}
	if err := s.saveSession(ctx, session, types.RunStatusCompleted); err != nil {
		_, _ = s.updateRun(record.runID, types.RunStatusFailed, "save session failed: "+err.Error())
		s.publish(record.runID, "run_failed", types.RunState{ID: record.runID, Status: types.RunStatusFailed, Reason: "save session failed"})
		return
	}
	s.publish(record.runID, "run_completed", state)
}

func (s *system) failRun(ctx context.Context, record *runRecord, session types.Session, reason string) {
	state, err := s.updateRun(record.runID, types.RunStatusFailed, reason)
	if err != nil {
		return
	}
	if session.ID != "" {
		session = appendMessage(session, failureMessage(reason))
		if err := s.saveSession(ctx, session, types.RunStatusFailed); err != nil {
			s.publish(record.runID, "run_failed", types.RunState{ID: record.runID, Status: types.RunStatusFailed, Reason: "save session failed: " + err.Error()})
			return
		}
	}
	s.publish(record.runID, "run_failed", state)
}

func (s *system) cancelRunRecord(ctx context.Context, record *runRecord, session types.Session) {
	state, err := s.updateRun(record.runID, types.RunStatusCancelled, "cancelled")
	if err != nil {
		return
	}
	if session.ID != "" {
		if err := s.saveSession(ctx, session, types.RunStatusCancelled); err != nil {
			s.publish(record.runID, "run_failed", types.RunState{ID: record.runID, Status: types.RunStatusFailed, Reason: "save session failed: " + err.Error()})
			return
		}
	}
	s.publish(record.runID, "run_cancelled", state)
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
