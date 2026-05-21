package api

import (
	"server/internal/app/contracts"

	"github.com/gin-gonic/gin"
)

const servicesContextKey = "api_services"

func servicesMiddleware(s *contracts.APIServices) gin.HandlerFunc {
	if err := validateAPIServices(s); err != nil {
		panic("api services are not configured: " + err.Error())
	}

	return func(c *gin.Context) {
		c.Set(servicesContextKey, s)
		c.Next()
	}
}

func servicesFromContext(c *gin.Context) *contracts.APIServices {
	if c == nil {
		panic("api services are not configured: nil gin context")
	}

	if value, ok := c.Get(servicesContextKey); ok {
		if services, ok := value.(*contracts.APIServices); ok && services != nil {
			return services
		}
	}

	panic("api services are not configured in gin context")
}
