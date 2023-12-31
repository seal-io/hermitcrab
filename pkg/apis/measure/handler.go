package measure

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/seal-io/hermitcrab/pkg/apis/runtime"
	"github.com/seal-io/hermitcrab/pkg/health"
	"github.com/seal-io/hermitcrab/pkg/metric"
)

func Readyz() runtime.Handle {
	return func(c *gin.Context) {
		d, ok := health.MustValidate(c, []string{"database"})
		if !ok {
			c.String(http.StatusServiceUnavailable, d)
			return
		}

		c.String(http.StatusOK, d)
	}
}

func Livez() runtime.Handle {
	return func(c *gin.Context) {
		d, ok := health.Validate(c, c.QueryArray("exclude")...)
		if !ok {
			c.String(http.StatusServiceUnavailable, d)
			return
		}

		c.String(http.StatusOK, d)
	}
}

func Metrics() runtime.HTTPHandler {
	return metric.Index(5, 30*time.Second)
}
