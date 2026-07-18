package gateway

import (
	"context"
	"net/http"
	"strings"
)

type relayContextKey int

const (
	relayAccountKey relayContextKey = iota
	relayStartedKey
)

// RelayAccount runs the normal Amber gateway pipeline while constraining
// account selection to a cloud-authorized client UID. The callback fires
// immediately before the first upstream network request.
func (e *Engine) RelayAccount(w http.ResponseWriter, r *http.Request, accountUID string, upstreamStarted func()) {
	ctx := context.WithValue(r.Context(), relayAccountKey, strings.TrimSpace(accountUID))
	if upstreamStarted != nil {
		ctx = context.WithValue(ctx, relayStartedKey, upstreamStarted)
	}
	r = r.WithContext(ctx)
	if strings.HasSuffix(r.URL.Path, "/chat/completions") {
		e.ChatCompletions(w, r)
		return
	}
	e.Responses(w, r)
}

func relayAccountUID(ctx context.Context) string {
	value, _ := ctx.Value(relayAccountKey).(string)
	return value
}

func markRelayUpstreamStarted(ctx context.Context) {
	if callback, ok := ctx.Value(relayStartedKey).(func()); ok && callback != nil {
		callback()
	}
}
