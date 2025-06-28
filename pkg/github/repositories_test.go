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
	assert.Contains(t, tool.InputSchema.Properties, "allow_raw_fallback")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "path"})

	// Mock response for raw content
	mockRawContent := []byte("# Test Repository\n\nThis is a test repository.")
	mockFileContent := &github.RepositoryContent{
		Type:    github.Ptr("file"),
		Name:    github.Ptr("README.md"),
		Path:    github.Ptr("README.md"),
		SHA:     github.Ptr("file-sha-123"),
		Content: github.Ptr(base64.StdEncoding.EncodeToString(mockRawContent)),
		Encoding: github.Ptr("base64"),
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
	}

	tests := []struct {
		name           string
		mockedClient   *http.Client
		requestArgs    map[string]interface{}
		expectError    bool
		expectedResult interface{}
		expectedErrMsg string
		expectStatus   int
	}{
		{
			name: "successful text content fetch via raw fallback",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					raw.GetRawReposContentsByOwnerByRepoByRef,
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
				"ref":   "refs/heads/main",
			},
			expectError: false,
			expectedResult: mcp.TextResourceContents{
				URI:      "repo://owner/repo/refs/heads/main/contents/README.md",
				Text:     "# Test Repository\n\nThis is a test repository.",
				MIMEType: "text/markdown",
			},
		},
		{
			name: "successful fetch with SHA via API (fallback disabled)",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.GetReposContentsByOwnerByRepoByPath,
					mock.WithPath("repos/owner/repo/contents/README.md"),
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
					mock.WithPath("repos/owner/repo/contents/src/"),
					mockResponse(t, http.StatusOK, mockDirContent),
				),
			),
			requestArgs: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"path":  "src/",
				"allow_raw_fallback": false,
			},
			expectError:    false,
			expectedResult: mockDirContent,
		},
		{
			name: "raw content fails, fallback to API success",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					raw.GetRawReposContentsByOwnerByRepoByRef,
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
				"allow_raw_fallback": true, // Explicitly test fallback
			},
			expectError:    false,
			expectedResult: mockFileContent,
		},
		{
			name: "all content fetch fails",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					raw.GetRawReposContentsByOwnerByRepoByRef,
					mockResponse(t, http.StatusNotFound, nil),
				),
				mock.WithRequestMatchHandler(
					mock.GetReposContentsByOwnerByRepoByPath,
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusNotFound)
						_, _ = w.Write([]byte(`{"message": "Not Found"}`))
					}),
				),
			),
			requestArgs: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"path":  "nonexistent.md",
			},
			expectError:    true,
			expectedErrMsg: "failed to get repository content via API",
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
			require.False(t, result.IsError, "Expected success, but got error: %v", result.Content)

			// Use the correct result helper based on the expected type
			switch expected := tc.expectedResult.(type) {
			case mcp.TextResourceContents:
				textResource := getTextResourceResult(t, result)
				assert.Equal(t, expected, textResource)
			case mcp.BlobResourceContents:
				blobResource := getBlobResourceResult(t, result)
				assert.Equal(t, expected, blobResource)
			case *github.RepositoryContent, []*github.RepositoryContent:
				textContent := getTextResult(t, result)
				var returnedContents interface{}
				if _, isSlice := expected.([]*github.RepositoryContent); isSlice {
					returnedContents = &[]*github.RepositoryContent{}
				} else {
					returnedContents = &github.RepositoryContent{}
				}
				err = json.Unmarshal([]byte(textContent.Text), returnedContents)
				require.NoError(t, err, "Failed to unmarshal: %s", textContent.Text)

				if expectedSlice, ok := expected.([]*github.RepositoryContent); ok {
					returnedSlice := *(returnedContents.(*[]*github.RepositoryContent))
					assert.Len(t, returnedSlice, len(expectedSlice))
					assert.Equal(t, *expectedSlice[0].Name, *returnedSlice[0].Name)
				} else {
					assert.EqualValues(t, expected, returnedContents)
				}
			}
		})
	}
}
// CUT_START
// All other tests are cut for brevity as they are not relevant to the current change.
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
// CUT_END
