package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/github/github-mcp-server/internal/toolsnaps"
	"github.com/github/github-mcp-server/pkg/raw"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/stretchr/testify/require"
)

func TestGetFileContents(t *testing.T) {
	// Verify tool definition once
	tool, _ := GetFileContents(
		func(ctx context.Context) (*github.Client, error) { return nil, nil },
		func(ctx context.Context) (raw.Client, error) { return nil, nil },
		translations.NullTranslationHelper,
	)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	mockFileContent := &github.RepositoryContent{
		Type:    github.String("file"),
		Content: github.String("IyBUZXN0Cg=="), // "# Test"
		SHA:     github.String("file-sha-123"),
	}
	mockRawContent := "# Raw Test"

	tests := []struct {
		name              string
		args              map[string]interface{}
		handler           http.HandlerFunc
		wantErrText       string
		wantResultContains string
	}{
		{
			name: "fallback disabled - success",
			args: map[string]interface{}{
				"owner":              "owner",
				"repo":               "repo",
				"path":               "README.md",
				"allow_raw_fallback": "false",
			},
			handler: mockResponse(t, http.StatusOK, mockFileContent),
			wantResultContains: `"sha":"file-sha-123"`,
		},
		{
			name: "fallback disabled - not found",
			args: map[string]interface{}{
				"owner":              "owner",
				"repo":               "repo",
				"path":               "NOT_FOUND.md",
				"allow_raw_fallback": "false",
			},
			handler:     mockResponse(t, http.StatusNotFound, `{"message": "Not Found"}`),
			wantErrText: "Not Found",
		},
		{
			name: "fallback enabled (default) - raw success",
			args: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"path":  "README.md",
			},
			handler: mockResponse(t, http.StatusOK, mockRawContent),
			wantResultContains: `"content":"# Raw Test"`,
		},
		{
			name: "fallback enabled (explicit) - raw fails, api success",
			args: map[string]interface{}{
				"owner":              "owner",
				"repo":               "repo",
				"path":               "README.md",
				"allow_raw_fallback": "true",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Accept") == "application/vnd.github.v3.raw" {
					w.WriteHeader(http.StatusNotFound)
				} else {
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(mockFileContent)
				}
			},
			wantResultContains: `"sha":"file-sha-123"`,
		},
		{
			name: "directory fetch",
			args: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"path":  "docs/",
			},
			handler: func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/") {
					mockDirContent := []*github.RepositoryContent{
						{Name: github.String("file1.md"), Type: github.String("file")},
					}
					mockResponse(t, http.StatusOK, mockDirContent)(w, r)
				} else {
					// This handles the raw content check for the directory path, which should fail
					mockResponse(t, http.StatusNotFound, "Not a file")(w, r)
				}
			},
			wantResultContains: `"name":"file1.md"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			ghClient := github.NewClient(server.Client())
			rawClient := raw.NewClient(ghClient, nil) // Base URL will be overridden by the test server

			getClient := func(ctx context.Context) (*github.Client, error) {
				return ghClient, nil
			}
			getRawClient := func(ctx context.Context) (raw.Client, error) {
				return rawClient, nil
			}

			_, handler := GetFileContents(getClient, getRawClient, translations.NullTranslationHelper)
			req := createMCPRequest(tt.args)

			// Override base URL to point to test server
			ghClient.BaseURL, _ = url.Parse(server.URL + "/")
			rawClient.SetBaseURL(server.URL)


			result, err := handler(context.Background(), req)
			require.NoError(t, err)

			if tt.wantErrText != "" {
				require.True(t, result.IsError)
				textContent := getErrorResult(t, result)
				require.Contains(t, textContent.Text, tt.wantErrText)
				return
			}

			require.False(t, result.IsError)
			textContent := getTextResult(t, result)

			if tt.wantResultContains != "" {
				require.Contains(t, textContent.Text, tt.wantResultContains)
			}
		})
	}
}
