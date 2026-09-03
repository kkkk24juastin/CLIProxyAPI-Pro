package auth

import (
	"context"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	cliproxysession "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/session"
)

// ExecutePinnedAuth executes one non-streaming request through the exact auth
// record identified by authID. Unlike normal scheduling, this diagnostic path
// deliberately bypasses disabled, cooldown, and unavailable eligibility gates
// so an operator can verify whether a credential has recovered. The normal
// preparation, proxy transport, unauthorized refresh, alias mapping, and result
// accounting paths are still applied.
func (m *Manager) ExecutePinnedAuth(ctx context.Context, authID string, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	if m == nil {
		return cliproxyexecutor.Response{}, &Error{Code: "auth_not_found", Message: "auth manager is unavailable"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	req, opts = cliproxysession.Enrich(req, opts)
	auth, okAuth := m.GetByID(authID)
	if !okAuth || auth == nil {
		return cliproxyexecutor.Response{}, &Error{Code: "auth_not_found", Message: "auth not found"}
	}
	provider := executorKeyFromAuth(auth)
	executor, okExecutor := m.Executor(provider)
	if !okExecutor || executor == nil {
		return cliproxyexecutor.Response{}, &Error{Code: "executor_not_found", Message: "executor not registered"}
	}

	routeModel := authSelectionModelFromOptions(opts, req.Model)
	executionModel, restoreExecutionModel := executionModelForAuthSelection(opts, req.Model)
	opts = ensureRequestedModelMetadata(opts, routeModel)
	execCtx := ctx
	if rt := m.roundTripperFor(auth); rt != nil {
		execCtx = context.WithValue(execCtx, roundTripperContextKey{}, rt)
		execCtx = context.WithValue(execCtx, "cliproxy.roundtripper", rt)
	}
	execCtx = contextWithRequestedModelAlias(execCtx, opts, routeModel)

	models, pooled, aliasResult, routing := m.executionModelCandidatesWithAlias(auth, routeModel)
	if len(models) == 0 {
		return cliproxyexecutor.Response{}, &Error{Code: "model_not_found", Message: "auth does not provide the requested model"}
	}

	preparedAuth, errPrepare := m.prepareRequestAuth(execCtx, executor, auth)
	if errPrepare != nil {
		result := Result{AuthID: auth.ID, Provider: provider, Model: routeModel, Success: false, Error: resultErrorFromError(errPrepare)}
		m.MarkResult(execCtx, result)
		return cliproxyexecutor.Response{}, errPrepare
	}
	auth = preparedAuth

	var lastErr error
	didRefreshOnUnauthorized := false
	for _, upstreamModel := range models {
		resultModel := m.stateModelForExecution(auth, routeModel, upstreamModel, pooled)
		execReq := req
		execReq.Model = upstreamModel
		if restoreExecutionModel {
			execReq.Model = executionModel
		}
		execOpts := opts
		var errIntercept error
		execReq, execOpts, errIntercept = applyRequestAfterAuthInterceptor(execCtx, executor, provider, execReq, execOpts, requestedModelAliasFromOptions(execOpts, routeModel))
		if errIntercept != nil {
			return cliproxyexecutor.Response{}, errIntercept
		}
		if !restoreExecutionModel {
			execReq = attachResolvedAPIKeyModelInfo(routing, execReq, auth, routeModel, upstreamModel)
		}

		resp, errExecute := executor.Execute(execCtx, auth, execReq, execOpts)
		if errExecute != nil {
			if errContext := execCtx.Err(); errContext != nil {
				return cliproxyexecutor.Response{}, errContext
			}
			if refreshed, okRefresh := m.tryRefreshAfterUnauthorized(execCtx, auth, errExecute, didRefreshOnUnauthorized); okRefresh {
				auth = refreshed
				didRefreshOnUnauthorized = true
				resp, errExecute = executor.Execute(execCtx, auth, execReq, execOpts)
			}
		}
		if errCancel := claudeOAuthRequestCancellation(execCtx, auth, errExecute); errCancel != nil {
			return cliproxyexecutor.Response{}, errCancel
		}

		result := Result{AuthID: auth.ID, Provider: provider, Model: resultModel, Success: errExecute == nil}
		if errExecute != nil {
			result.Error = resultErrorFromError(errExecute)
			if retryAfter := retryAfterFromError(errExecute); retryAfter != nil {
				result.RetryAfter = retryAfter
			}
			m.MarkResult(execCtx, result)
			if isRequestInvalidError(errExecute) {
				return cliproxyexecutor.Response{}, errExecute
			}
			lastErr = errExecute
			continue
		}

		m.MarkResult(execCtx, result)
		attemptAliasResult := resolveAttemptAliasResult(routing, auth, routeModel, upstreamModel, aliasResult)
		rewriteForceMappedResponse(&resp, attemptAliasResult)
		return resp, nil
	}
	if lastErr != nil {
		return cliproxyexecutor.Response{}, lastErr
	}
	return cliproxyexecutor.Response{}, &Error{Code: "model_not_found", Message: "auth does not provide the requested model"}
}
