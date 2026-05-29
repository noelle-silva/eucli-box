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
	record := &runRecord{state: state, cancel: cancel}
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
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.runs[runID]
	if !ok {
		return types.RunState{}, runtimeNotFound("run was not found", nil)
	}
	return record.state, nil
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
	state, err := s.updateRun(record.state.ID, types.RunStatusRunning, "")
	if err != nil {
		return
	}
	s.publish(record.state.ID, "run_started", state)
	session, err := s.loadOrCreateSession(ctx, request)
	if err != nil {
		s.failRun(context.Background(), record, session, err.Error())
		return
	}
	if record.state.SessionID == "" {
		if err := s.setRunSessionID(record.state.ID, session.ID); err != nil {
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
			s.cancelRunRecord(context.Background(), record, session)
			return
		}
		roleContext, err := s.buildRoleContext(ctx, record.state.RoleID, session)
		if err != nil {
			s.failRun(context.Background(), record, session, err.Error())
			return
		}
		modelResponse, err := s.callModel(ctx, roleContext)
		if err != nil {
			s.failRun(context.Background(), record, session, err.Error())
			return
		}
		session = appendMessage(session, assistantMessage(modelResponse.Content))
		s.publish(record.state.ID, "model_output", modelResponse)
		if err := s.saveSession(ctx, session, types.RunStatusRunning); err != nil {
			s.failRun(context.Background(), record, session, err.Error())
			return
		}
		if len(modelResponse.ToolIntents) == 0 {
			s.completeRun(context.Background(), record, session)
			return
		}
		if len(modelResponse.ToolIntents) > 1 {
			s.failRun(context.Background(), record, session, "model returned more than one tool intent")
			return
		}
		result, _, err := s.handleToolIntent(ctx, record, modelResponse.ToolIntents[0])
		if err != nil {
			s.failRun(context.Background(), record, session, err.Error())
			return
		}
		session = appendMessage(session, toolMessage(result))
		s.publish(record.state.ID, "tool_result", result)
		if err := s.saveSession(ctx, session, types.RunStatusRunning); err != nil {
			s.failRun(context.Background(), record, session, err.Error())
			return
		}
	}
	s.failRun(context.Background(), record, session, "run exceeded max steps")
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
	state, err := s.updateRun(record.state.ID, types.RunStatusCompleted, "")
	if err != nil {
		return
	}
	_ = s.saveSession(ctx, session, types.RunStatusCompleted)
	s.publish(record.state.ID, "run_completed", state)
}

func (s *system) failRun(ctx context.Context, record *runRecord, session types.Session, reason string) {
	state, err := s.updateRun(record.state.ID, types.RunStatusFailed, reason)
	if err != nil {
		return
	}
	if session.ID != "" {
		session = appendMessage(session, failureMessage(reason))
		_ = s.saveSession(ctx, session, types.RunStatusFailed)
	}
	s.publish(record.state.ID, "run_failed", state)
}

func (s *system) cancelRunRecord(ctx context.Context, record *runRecord, session types.Session) {
	state, err := s.updateRun(record.state.ID, types.RunStatusCancelled, "cancelled")
	if err != nil {
		return
	}
	if session.ID != "" {
		_ = s.saveSession(ctx, session, types.RunStatusCancelled)
	}
	s.publish(record.state.ID, "run_cancelled", state)
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
