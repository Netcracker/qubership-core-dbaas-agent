package controller

import (
	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

func (controller *Controller) HandleGetHealth(c *fiber.Ctx) error {
	userCtx := c.UserContext()
	resp, err := controller.forwarder.DoRequest(userCtx, fasthttp.MethodGet, "/health", nil)
	defer fasthttp.ReleaseResponse(resp)
	if err != nil {
		logger.ErrorC(userCtx, "Error during forwarding request to dbaas-aggregator: %v", err)
		return respondWithError(userCtx, c, fiber.StatusInternalServerError, "Error happened during forwarding request to DBaaS")
	}
	return returnDbaasResponse(userCtx, c, resp)
}

func (controller *Controller) HandleProbes(c *fiber.Ctx) error {
	c.Status(fasthttp.StatusOK)
	return nil
}
