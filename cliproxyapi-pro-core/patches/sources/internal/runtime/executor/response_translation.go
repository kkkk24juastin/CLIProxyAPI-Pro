package executor

import (
	"context"
	"fmt"
	"net/http"

	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
)

// translateNonStreamResponse turns translator panics into an ordinary executor
// error. Executors can then publish the buffered upstream usage as a failed
// terminal record instead of allowing an earlier success publication to win.
func translateNonStreamResponse(
	ctx context.Context,
	from sdktranslator.Format,
	to sdktranslator.Format,
	model string,
	originalRequest []byte,
	translatedRequest []byte,
	response []byte,
	param *any,
) (out []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			out = nil
			err = statusErr{
				code: http.StatusBadGateway,
				msg:  fmt.Sprintf("response translation failed: %v", recovered),
			}
		}
	}()
	out = sdktranslator.TranslateNonStream(ctx, from, to, model, originalRequest, translatedRequest, response, param)
	return out, nil
}
