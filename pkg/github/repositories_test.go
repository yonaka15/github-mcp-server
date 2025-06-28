package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/github/github-mcp-server/internal/toolsnaps"
	"github.com/github/github-mcp-server/pkg/raw"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v72/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetFileContents(t *testing.T) {
	// Verify tool definition once
	mockClient := github.NewClient(nil)
	mockRawClient := raw.NewClient(mockClient, &url.URL{Scheme: "https", Host: "raw.githubusercontent.com", Path: "/"})
	tool, _ := GetFileContents(stubGetClientFn(mockClient), stubGetRawClientFn(mockRawClient), translations.NullTranslationHelper)
	require.NoError(t, toolsnaps.Test(tool.Name, tool))

	assert.Equal(t, "get_file_contents", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "path")
	assert.Contains(t, tool.InputSchema.Properties, "ref")
	assert.Contains(t, tool.InputSchema.Properties, "sha")
	assert.Contains(t, tool.InputSchema.Properties, "allow_raw_fallback")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "path"})

	// Mock response for raw content
	mockRawContent := []byte("# Test Repository\n\nThis is a test repository.")
	
	// Mock response for GetContents API (file)
	mockFileContent := &github.RepositoryContent{
		Type:        github.Ptr("file"),
		Name:        github.Ptr("README.md"),
		Path:        github.Ptr("README.md"),
		SHA:         github.Ptr("file-sha-123"),
		Content:     github.Ptr(base64.StdEncoding.EncodeToString(mockRawContent) + "\n"),
		DownloadURL: github.Ptr("https://raw.githubusercontent.com/owner/repo/main/README.md"),
	}

	// Setup mock directory content for success case
	mockDirContent := []*github.RepositoryContent{
		{
			Type:    github.Ptr("file"),
			Name:    github.Ptr("README.md"),
			Path:    github.Ptr("README.md"),
			SHA:     github.Ptr("abc123"),
			Size:    github.Ptr(42),
			HTMLURL: github.Ptr("https://github.com/owner/repo/blob/main/README.md"),
		},
		{
			Type:    github.Ptr("dir"),
			Name:    github.Ptr("src"),
			Path:    github.Ptr("src"),
			SHA:     github.Ptr("def456"),
			HTMLURL: github.Ptr("https://github.com/owner/repo/tree/main/src"),
		},
	}

	tests := []struct {
		name           string
		mockedClient   *http.Client
		requestArgs    map[string]interface{}
		expectError    bool
		expectedResult interface{}
		expectedErrMsg string
	}{
		{
			name: "successful text content fetch via raw fallback (default)",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.EndpointPattern{
						Method: "GET",
						Host:   "raw.example.com",
						Path:   "/owner/repo/HEAD/README.md",
					},
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set("Content-Type", "text/markdown")
						_, _ = w.Write(mockRawContent)
					}),
				),
			),
			requestArgs: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"path":  "README.md",
			},
			expectError: false,
			expectedResult: mcp.TextResourceContents{
				URI:      "repo://owner/repo/contents/README.md",
				Text:     string(mockRawContent),
				MIMEType: "text/markdown",
			},
		},
		{
			name: "successful content fetch via GetContents API (fallback disabled)",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposContentsByOwnerByRepoByPath,
					mockResponse(t, http.StatusOK, mockFileContent),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":              "owner",
				"repo":               "repo",
				"path":               "README.md",
				"allow_raw_fallback": false,
			},
			expectError:    false,
			expectedResult: mockFileContent,
		},
		{
			name: "successful directory content fetch (fallback disabled)",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposContentsByOwnerByRepoByPath,
					mockResponse(t, http.StatusOK, mockDirContent),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":              "owner",
				"repo":               "repo",
				"path":               "src/",
				"allow_raw_fallback": false,
			},
			expectError:    false,
			expectedResult: mockDirContent,
		},
		{
			name: "raw content fails, fallback to GetContents API succeeds",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.EndpointPattern{
						Method: "GET",
						Host:   "raw.example.com",
					},
					mockResponse(t, http.StatusNotFound, nil),
				),
				mock.WithRequestMatchHandler(
					mock.GetReposContentsByOwnerByRepoByPath,
					mockResponse(t, http.StatusOK, mockFileContent),
				),
			),
			requestArgs: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"path":  "README.md",
			},
			expectError:    false,
			expectedResult: mockFileContent,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup client with mock
			client := github.NewClient(tc.mockedClient)
			mockRawClient := raw.NewClient(client, &url.URL{Scheme: "https", Host: "raw.example.com", Path: "/"})
			_, handler := GetFileContents(stubGetClientFn(client), stubGetRawClientFn(mockRawClient), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError {
				require.NoError(t, err)
				require.True(t, result.IsError)
				errorContent := getErrorResult(t, result)
				assert.Contains(t, errorContent.Text, tc.expectedErrMsg)
				return
			}
			
			require.NoError(t, err)
			require.False(t, result.IsError, "Expected success but got error: %v", result)

			// Use the correct result helper based on the expected type
			switch expected := tc.expectedResult.(type) {
			case mcp.TextResourceContents:
				textResource := getTextResourceResult(t, result)
				assert.Equal(t, expected.URI, textResource.URI)
				assert.Equal(t, expected.Text, textResource.Text)
				assert.Equal(t, expected.MIMEType, textResource.MIMEType)
			case *github.RepositoryContent:
                 textContent := getTextResult(t, result)
                 var returnedContent *github.RepositoryContent
                 err = json.Unmarshal([]byte(textContent.Text), &returnedContent)
                 require.NoError(t, err)
                 assert.Equal(t, expected, returnedContent)
			case []*github.RepositoryContent:
				textContent := getTextResult(t, result)
				var returnedContents []*github.RepositoryContent
				err = json.Unmarshal([]byte(textContent.Text), &returnedContents)
				require.NoError(t, err)
				assert.Equal(t, expected, returnedContents)
			default:
				t.Fatalf("unhandled expected result type: %T", tc.expectedResult)
			}
		})
	}
}

// ... (the rest of the test file remains the same)
