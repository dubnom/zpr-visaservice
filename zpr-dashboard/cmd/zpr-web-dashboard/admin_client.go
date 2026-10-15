package main

import (
	"context"
	"errors"
	"net/http"
	"os"

	"neboagency.com/zpr-dashborad/internal/controlservice/adminapi"
)

type adminClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type adminStatusError struct {
	code   int
	status string
}

func (e adminStatusError) Error() string { return "admin API returned " + e.status }

func newAdminClient() (*adminClient, string) {
	client, err := adminapi.New(adminapi.Config{
		BaseURL:    os.Getenv("ZPR_ADMIN_URL"),
		APIKey:     os.Getenv("ZPR_ADMIN_API_KEY"),
		KeyFile:    os.Getenv("ZPR_ADMIN_KEY_FILE"),
		CAFile:     os.Getenv("ZPR_ADMIN_CA_FILE"),
		ServerName: os.Getenv("ZPR_ADMIN_SERVER_NAME"),
	})
	switch {
	case errors.Is(err, adminapi.ErrMissingConfig):
		return nil, "set ZPR_ADMIN_URL, ZPR_ADMIN_CA_FILE, and either ZPR_ADMIN_API_KEY or ZPR_ADMIN_KEY_FILE"
	case errors.Is(err, adminapi.ErrInvalidURL):
		return nil, "ZPR_ADMIN_URL must be a valid https URL"
	case errors.Is(err, adminapi.ErrURLPath):
		return nil, "ZPR_ADMIN_URL must contain only the scheme and host"
	case err != nil:
		return nil, err.Error()
	}
	return &adminClient{baseURL: client.BaseURL, apiKey: client.APIKey, http: client.HTTP}, ""
}

func (c *adminClient) getJSON(ctx context.Context, path string, target any) error {
	err := c.reader().GetJSON(ctx, path, target)
	var status adminapi.StatusError
	if errors.As(err, &status) {
		return adminStatusError{code: status.Code, status: status.Status}
	}
	return err
}

func (c *adminClient) reader() *adminapi.Client {
	return &adminapi.Client{BaseURL: c.baseURL, APIKey: c.apiKey, HTTP: c.http}
}
