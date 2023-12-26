package runtime

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/seal-io/walrus/utils/errorx"
	"github.com/seal-io/walrus/utils/log"
)

// erroring is a gin middleware,
// which converts the chain calling error into response.
func erroring(c *gin.Context) {
	c.Next()

	if len(c.Errors) == 0 {
		if c.Writer.Status() >= http.StatusBadRequest && c.Writer.Size() == 0 {
			// Detail the error status message.
			_ = c.Error(errorx.NewHttpError(c.Writer.Status(), ""))
		} else {
			// No errors.
			return
		}
	}

	// Get errors from chain and parse into response.
	he := getHttpError(c)

	// Log errors.
	if len(he.errs) != 0 && withinStacktraceStatus(he.Status) {
		reqMethod := c.Request.Method

		reqPath := c.Request.URL.Path
		if raw := c.Request.URL.RawQuery; raw != "" {
			reqPath = reqPath + "?" + raw
		}

		log.WithName("api").
			Errorf("error requesting %s %s: %v", reqMethod, reqPath, errorx.Format(he.errs))
	}

	c.AbortWithStatusJSON(he.Status, he)
}

func getHttpError(c *gin.Context) (he ErrorResponse) {
	var errs []error

	for i := range c.Errors {
		if c.Errors[i].Err != nil {
			errs = append(errs, c.Errors[i].Err)
		}
	}
	he.errs = errs

	if len(errs) == 0 {
		he.Status = http.StatusInternalServerError
	} else {
		// Get the public error.
		he.Status, he.Message = errorx.Public(errs)

		// Get the last error.
		if he.Status == 0 {
			st, msg := diagnoseError(c.Errors.Last())
			he.Status = st

			if he.Message == "" {
				he.Message = msg
			}
		}
	}

	// Correct the code if already write within context.
	if c.Writer.Written() {
		he.Status = c.Writer.Status()
	}

	he.StatusText = http.StatusText(he.Status)

	return
}

type ErrorResponse struct {
	Message    string `json:"message"`
	Status     int    `json:"status"`
	StatusText string `json:"statusText"`

	// Errs is the all errors from gin context errors.
	errs []error
}

func diagnoseError(ge *gin.Error) (int, string) {
	c := http.StatusInternalServerError
	if ge.Type == gin.ErrorTypeBind {
		c = http.StatusBadRequest
	}

	var b strings.Builder

	if ge.Meta != nil {
		m, ok := ge.Meta.(string)
		if ok {
			b.WriteString("failed to ")
			b.WriteString(m)
		}
	}

	err := ge.Err
	if ue := errors.Unwrap(err); ue != nil {
		err = ue
	}

	// TODO: distinguish between internal and external errors.
	b.WriteString(err.Error())

	return c, b.String()
}

func withinStacktraceStatus(status int) bool {
	return (status < http.StatusOK || status >= http.StatusInternalServerError) &&
		status != http.StatusSwitchingProtocols
}
