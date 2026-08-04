// Package internalusage preserves the historical embeddedusage event types.
package internalusage

import prointernalusage "github.com/router-for-me/CLIProxyAPI/v7/internal/pro/observability/internalusage"

type (
	Event          = prointernalusage.Event
	Tokens         = prointernalusage.Tokens
	Detail         = prointernalusage.Detail
	ModelAggregate = prointernalusage.ModelAggregate
	APIAggregate   = prointernalusage.APIAggregate
	Payload        = prointernalusage.Payload
)

var (
	NormalizeRaw             = prointernalusage.NormalizeRaw
	BuildPayload             = prointernalusage.BuildPayload
	NormalizeEventAccounting = prointernalusage.NormalizeEventAccounting
)
