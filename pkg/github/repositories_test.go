package github

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/github/github-mcp-server/internal/toolsnaps"
	"github.com/github/github-mcp-server/pkg/raw"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/migueleliasweb/go-github-mock/mock"
	"github.com/stretchr/testify/require"
)

func TestGetFileContents(t *testing.T) {
	mockFileContent := &github.RepositoryContent{
		Type:    github.String("file"),
		Content: github.String("IyBUZXN0Cg=="), // "# Test"
		SHA:     github.String("file-sha-123"),
	}
	mockRawContent := "# Raw Test"

	// Verify tool definition once
	tool, _ := GetFileContents(
		func(ctx context.Context) (*github.Client, error) { return nil, nil },
		func(ctx context.Context) (raw.Client, error) { return nil, nil },
		translations.NullTranslationHelper,
	)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	tests := []struct {
		name              string
		args              map[string]interface{}
		mockSetup         func(mocked *mock.Mock)
		wantErrText       string
		wantResult      string
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
			mockSetup: func(mocked *mock.Mock) {
				mock.RegisterFromJSON(
					mocked,
					http.MethodGet,
					"/repos/owner/repo/contents/README.md",
					http.StatusOK,
					mockFileContent,
				)
			},
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
			mockSetup: func(mocked *mock.Mock) {
				mock.RegisterFromJSON(
					mocked,
					http.MethodGet,
					"/repos/owner/repo/contents/NOT_FOUND.md",
					http.StatusNotFound,
					&github.ErrorResponse{Message: "Not Found"},
				)
			},
			wantErrText: "Not Found",
		},
		{
			name: "fallback enabled (default) - raw success",
			args: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"path":  "README.md",
			},
			mockSetup: func(mocked *mock.Mock) {
				mocked.Register(
					mock.EndpointPattern(http.MethodGet, "/repos/owner/repo/contents/README.md"),
					mock.WithHeader("Accept", "application/vnd.github.v3.raw"),
					mock.Status(http.StatusOK),
					mock.Body(mockRawContent),
				)
			},
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
			mockSetup: func(mocked *mock.Mock) {
				mocked.Register(
					mock.EndpointPattern(http.MethodGet, "/repos/owner/repo/contents/README.md"),
					mock.WithHeader("Accept", "application/vnd.github.v3.raw"),
					mock.Status(http.StatusNotFound),
				)
				mock.RegisterFromJSON(
					mocked,
					http.MethodGet,
					"/repos/owner/repo/contents/README.md",
					http.StatusOK,
					mockFileContent,
				)
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
			mockSetup: func(mocked *mock.Mock) {
				mockDirContent := []*github.RepositoryContent{
					{Name: github.String("file1.md"), Type: github.String("file")},
				}
				mock.RegisterFromJSON(
					mocked,
					http.MethodGet,
					"/repos/owner/repo/contents/docs/",
					http.StatusOK,
					mockDirContent,
				)
			},
			wantResultContains: `"name":"file1.md"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mocked, rawMocked := mock.NewMockedClients()

			if tt.mockSetup != nil {
				tt.mockSetup(mocked)
			}

			getClient := func(ctx context.Context) (*github.Client, error) {
				return github.NewClient(mocked.GetClient()), nil
			}
			getRawClient := func(ctx context.Context) (raw.Client, error) {
				return raw.NewClient(github.NewClient(rawMocked.GetClient()), nil), nil
			}

			_, handler := GetFileContents(getClient, getRawClient, translations.NullTranslationHelper)
			req := createMCPRequest(tt.args)

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

			if tt.wantResult != "" {
				require.JSONEq(t, tt.wantResult, textContent.Text)
			}
			if tt.wantResultContains != "" {
				require.Contains(t, textContent.Text, tt.wantResultContains)
			}
		})
	}
}

func getResultMap(t *testing.T, result *mcp.CallToolResult) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(getTextResult(t, result).Text), &m))
	return m
}
