package controller

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v3"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/netcracker/qubership-core-lib-go/v3/security"
	"github.com/netcracker/qubership-core-lib-go/v3/serviceloader"
	"github.com/valyala/fasthttp"
)

var ErrUnauthorized = fiber.NewError(fiber.StatusUnauthorized, "controller: unauthorized request")

func readRequestBody(userCtx context.Context, c *fiber.Ctx) (body map[string]interface{}, bytes []byte, err error) {
	bodyBytes := c.Request().Body()
	var parsedBody map[string]interface{}
	if len(bodyBytes) == 0 {
		parsedBody = nil
	} else if err := c.BodyParser(&parsedBody); err != nil {
		logger.ErrorC(userCtx, "Failed to unmarshal request body: %v", err)
		return nil, nil, err
	}
	return parsedBody, bodyBytes, nil
}

// GetTokenFromRequest extracts authorization token from request header, validates and returns it if token is valid.
// If this function returns an error that means that error HTTP response was already sent to a client.
func (controller *Controller) GetTokenFromRequest(ctx context.Context, c *fiber.Ctx) (*jwt.Token, error) {
	errmsg := ""
	accessToken := ""
	authHeader := c.Get("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if strings.ToLower(parts[0]) != "bearer" {
			errmsg = "Authorization failed, Authorization header must start with Bearer"
		} else if len(parts) != 2 {
			errmsg = "Authorization failed, Authorization header must be \"Bearer: token\""
		} else {
			// token found for Bearer
			accessToken = parts[1]
		}
	} else {
		errmsg = fasthttp.StatusMessage(fasthttp.StatusUnauthorized)
	}
	if errmsg != "" {
		logger.ErrorC(ctx, errmsg)
		c.Set("WWW-Authenticate", "Basic realm=Restricted")
		return nil, ErrUnauthorized
	}
	tokenProvider := serviceloader.MustLoad[security.TokenProvider]()
	validatedToken, err := tokenProvider.ValidateToken(ctx, accessToken)
	if err != nil {
		logger.ErrorC(ctx, "Token validation failed")
		return nil, ErrUnauthorized
	}
	return validatedToken, nil
}

func returnDbaasResponse(logCtx context.Context, c *fiber.Ctx, dbaasResponse *fasthttp.Response) error {
	if dbaasResponse.StatusCode() >= 400 {
		logger.ErrorC(logCtx, "dbaas-aggregator respond with error %v. Response body: %v", dbaasResponse.StatusCode(), string(dbaasResponse.Body()))
	}
	err := RespondWithBytes(c, dbaasResponse.StatusCode(), dbaasResponse.Body())
	return err
}

func RespondWithBytes(ctx *fiber.Ctx, code int, response []byte) error {
	logger.DebugC(ctx.UserContext(), "Send response code: %v, body: %v", code, string(response))
	ctx.Response().Header.SetContentType("application/json")
	out := make([]byte, len(response))
	copy(out, response)
	return ctx.Status(code).Send(out)
}

func respondWithError(ctx context.Context, c *fiber.Ctx, code int, msg string) error {
	return respondWithJson(ctx, c, code, map[string]string{"error": msg})
}

func respondWithJson(ctx context.Context, c *fiber.Ctx, code int, payload interface{}) error {
	c.Response().Header.SetContentType("application/json")
	logger.DebugC(ctx, "Send response code: %v, body: %+v", code, payload)
	return c.Status(code).JSON(payload)
}
