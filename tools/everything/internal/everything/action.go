package everything

import (
	"fmt"

	"eucli-box/pkg/types"
)

// actionName identifies one callable action of the everything tool.
type actionName string

const (
	actionSearch    actionName = "search"
	actionAuthorize actionName = "authorize"
	actionIndex     actionName = "index"
)

// searchOnlyArgumentNames are the arguments that only the search action
// accepts. Authorize and index reject their presence explicitly.
var searchOnlyArgumentNames = []string{"query", "scopePath", "instanceName", "maxResults", "maxOutputChars"}

// parsedRequest is one validated tool request: the selected action plus the
// search request fields that carry the common and search-specific parameters.
// Search fills every field; index reuses the fields as its full-disk
// preparation parameters; authorize only uses the common description and
// timeout.
type parsedRequest struct {
	Action actionName
	searchRequest
}

// parseRequest selects the requested action and validates its arguments.
// A missing action defaults to search; any other value is an explicit error.
func parseRequest(input types.ToolExecutionInput, config Config) (parsedRequest, error) {
	action, err := parseActionName(input.Arguments)
	if err != nil {
		return parsedRequest{}, err
	}
	if action != actionSearch {
		if err := rejectSearchOnlyArguments(input.Arguments); err != nil {
			return parsedRequest{}, err
		}
	}
	switch action {
	case actionAuthorize:
		request, err := parseCommonRequest(input, config)
		if err != nil {
			return parsedRequest{}, err
		}
		return parsedRequest{Action: action, searchRequest: request}, nil
	case actionIndex:
		request, err := parseIndexRequest(input, config)
		if err != nil {
			return parsedRequest{}, err
		}
		return parsedRequest{Action: action, searchRequest: request}, nil
	default:
		request, err := parseSearchRequest(input, config)
		if err != nil {
			return parsedRequest{}, err
		}
		return parsedRequest{Action: actionSearch, searchRequest: request}, nil
	}
}

func parseActionName(args map[string]any) (actionName, error) {
	value, ok := args["action"]
	if !ok || value == nil {
		return actionSearch, nil
	}
	text, err := stringValue(value, "action")
	if err != nil {
		return "", err
	}
	selected := actionName(text)
	switch selected {
	case actionSearch, actionAuthorize, actionIndex:
		return selected, nil
	default:
		return "", fmt.Errorf("argument %q must be one of %q, %q, or %q", "action", actionSearch, actionAuthorize, actionIndex)
	}
}

func rejectSearchOnlyArguments(args map[string]any) error {
	for _, key := range searchOnlyArgumentNames {
		if _, ok := args[key]; ok {
			return fmt.Errorf("argument %q is accepted only by the %q action", key, actionSearch)
		}
	}
	return nil
}

// parseCommonRequest reads the arguments every action accepts: description and
// the caller-specified timeout.
func parseCommonRequest(input types.ToolExecutionInput, config Config) (searchRequest, error) {
	description, err := mergedString(input, "description")
	if err != nil {
		return searchRequest{}, err
	}
	timeoutMs, err := intArgument(input.Arguments, "timeoutMs", 0)
	if err != nil {
		return searchRequest{}, err
	}
	if timeoutMs < 0 {
		return searchRequest{}, fmt.Errorf("timeoutMs must not be negative")
	}
	return searchRequest{Description: description, TimeoutMs: timeoutMs, ConnectTimeoutMs: config.Limits.DefaultConnectTimeoutMs}, nil
}

// parseIndexRequest builds the full-disk preparation request of the index
// action. The full-disk action binds to the configured default instance.
func parseIndexRequest(input types.ToolExecutionInput, config Config) (searchRequest, error) {
	request, err := parseCommonRequest(input, config)
	if err != nil {
		return searchRequest{}, err
	}
	scope, err := resolveSearchScope(input.HostWorkingDirectory, "")
	if err != nil {
		return searchRequest{}, err
	}
	request.ScopeMode = scope.Mode
	request.ScopePaths = scope.DisplayPaths
	request.InstanceName = config.Runtime.DefaultInstanceName
	return request, nil
}
