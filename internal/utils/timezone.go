package utils

import (
	"log"
	"time"

	"github.com/labstack/echo/v4"
)

func SetTimeZone() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			timeZone := c.Request().Header.Get("X-Timezone")
			if timeZone == "" {
				timeZone = time.Local.String()
			}
			userTimeZone, err := time.LoadLocation(timeZone)
			if err != nil {
				log.Default().Printf("Error loading timezone: %s", err)
				return err
			}
			log.Default().Println("User timezone:", userTimeZone)
			c.Set("userTimeZone", userTimeZone)
			return next(c)
		}
	}
}

func GetTimezone(c echo.Context) *time.Location {
	if tz, ok := c.Get("userTimeZone").(*time.Location); ok {
		return tz
	}
	return time.UTC
}
