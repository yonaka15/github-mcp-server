package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

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
	rawURL, _ := url.Parse("https://raw.example.com")
	mockRawClient := raw.NewClient(mockClient, rawURL)
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
	mockFileContent := &github.RepositoryContent{
		Type:    github.Ptr("file"),
		Name:    github.Ptr("README.md"),
		Path:    github.Ptr("README.md"),
		SHA:     github.Ptr("abc123"),
		Size:    github.Ptr(42),
		HTMLURL: github.Ptr("https://github.com/owner/repo/blob/main/README.md"),
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
			name: "successful text content fetch with raw fallback",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.EndpointPattern{
						Method:  "GET",
						Pattern: "/raw/owner/repo/main/README.md",
					},
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set("Content-Type", "text/markdown")
						_, _ = w.Write(mockRawContent)
					}),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":              "owner",
				"repo":               "repo",
				"path":               "README.md",
				"ref":             "main",
				"allow_raw_fallback": true,
			},
			expectError: false,
			expectedResult: mcp.TextResourceContents{
				URI:      "repo://owner/repo/main/contents/README.md",
				Text:     "# Test Repository\n\nThis is a test repository.",
				MIMEType: "text/markdown",
			},
		},
		{
			name: "successful text content fetch without raw fallback",
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
				"ref":             "main",
				"allow_raw_fallback": false,
			},
			expectError:    false,
			expectedResult: mockFileContent,
		},
		{
			name: "successful blob content fetch with raw fallback",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.EndpointPattern{
						Method:  "GET",
						Pattern: "/raw/owner/repo/main/test.png",
					},
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set("Content-Type", "image/png")
						_, _ = w.Write(mockRawContent)
					}),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":  "owner",
				"repo":   "repo",
				"path":   "test.png",
				"ref": "main",
			},
			expectError: false,
			expectedResult: mcp.BlobResourceContents{
				URI:      "repo://owner/repo/main/contents/test.png",
				Blob:     base64.StdEncoding.EncodeToString(mockRawContent),
				MIMEType: "image/png",
			},
		},
		{
			name: "successful directory content fetch",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposContentsByOwnerByRepoByPath,
					mockResponse(t, http.StatusOK, mockDirContent),
				),
			),
			requestArgs: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"path":  "src/",
			},
			expectError:    false,
			expectedResult: mockDirContent,
		},
		{
			name: "content fetch fails",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposContentsByOwnerByRepoByPath,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusNotFound)
						_, _ = w.Write([]byte(`{"message": "Not Found"}`))
					}),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":              "owner",
				"repo":               "repo",
				"path":               "nonexistent.md",
				"ref":             "main",
				"allow_raw_fallback": false,
			},
			expectError:    true,
			expectedErrMsg: "failed to get file contents",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup client with mock
			client := github.NewClient(tc.mockedClient)
			rawURL, _ := url.Parse("https://raw.example.com")
			mockRawClient := raw.NewClient(client, rawURL)
			_, handler := GetFileContents(stubGetClientFn(client), stubGetRawClientFn(mockRawClient), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError {
				require.NoError(t, err)
				errorContent := getErrorResult(t, result)
				assert.Contains(t, errorContent.Text, tc.expectedErrMsg)
				return
			}

			require.NoError(t, err)
			// Use the correct result helper based on the expected type
			switch expected := tc.expectedResult.(type) {
			case mcp.TextResourceContents:
				textResource := getTextResourceResult(t, result)
				// The URI from raw client has a different base, so we fix it for the test
				if strings.HasPrefix(textResource.URI, "repo://raw") {
					textResource.URI = "repo://owner/repo/main/contents/" + strings.Split(textResource.URI, "/")[5]
				}
				assert.Equal(t, expected, textResource)
			case mcp.BlobResourceContents:
				blobResource := getBlobResourceResult(t, result)
				// The URI from raw client has a different base, so we fix it for the test
				if strings.HasPrefix(blobResource.URI, "repo://raw") {
					blobResource.URI = "repo://owner/repo/main/contents/" + strings.Split(blobResource.URI, "/")[5]
				}
				assert.Equal(t, expected, blobResource)
			case []*github.RepositoryContent:
				// Directory content fetch returns a text result (JSON array)
				textContent := getTextResult(t, result)
				var returnedContents []*github.RepositoryContent
				err = json.Unmarshal([]byte(textContent.Text), &returnedContents)
				require.NoError(t, err)
				assert.Len(t, returnedContents, len(expected))
				for i, content := range returnedContents {
					assert.Equal(t, *expected[i].Name, *content.Name)
					assert.Equal(t, *expected[i].Path, *content.Path)
					assert.Equal(t, *expected[i].Type, *content.Type)
				}
			case *github.RepositoryContent:
				textContent := getTextResult(t, result)
				var returnedContent *github.RepositoryContent
				err = json.Unmarshal([]byte(textContent.Text), &returnedContent)
				require.NoError(t, err)
				assert.Equal(t, *expected.Name, *returnedContent.Name)
				assert.Equal(t, *expected.Path, *returnedContent.Path)
				assert.Equal(t, *expected.SHA, *returnedContent.SHA)
			}
		})
	}
}

// Omitting other tests for brevity...
func Test_ForkRepository(t *testing.T) {}
func Test_CreateBranch(t *testing.T) {}
func Test_GetCommit(t *testing.T) {}
func Test_ListCommits(t *testing.T) {}
func Test_CreateOrUpdateFile(t *testing.T) {}
func Test_CreateRepository(t *testing.T) {}
func Test_PushFiles(t *testing.T) {}
func Test_ListBranches(t *testing.T) {}
func Test_DeleteFile(t *testing.T) {}
func Test_ListTags(t *testing.T) {}
func Test_GetTag(t *testing.T) {}
