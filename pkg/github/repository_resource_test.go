package github

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/google/go-github/v69/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_RepoContentsResourceHandler(t *testing.T) {
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
	expectedDirContent := []mcp.TextResourceContents{
		{
			URI:      "https://github.com/owner/repo/blob/main/README.md",
			MIMEType: "",
			Text:     "README.md",
		},
		{
			URI:      "https://github.com/owner/repo/tree/main/src",
			MIMEType: "text/directory",
			Text:     "src",
		},
	}

	mockFileContent := &github.RepositoryContent{
		Type:        github.Ptr("file"),
		Name:        github.Ptr("README.md"),
		Path:        github.Ptr("README.md"),
		Content:     github.Ptr("IyBUZXN0IFJlcG9zaXRvcnkKClRoaXMgaXMgYSB0ZXN0IHJlcG9zaXRvcnku"), // Base64 encoded "# Test Repository\n\nThis is a test repository."
		SHA:         github.Ptr("abc123"),
		Size:        github.Ptr(42),
		HTMLURL:     github.Ptr("https://github.com/owner/repo/blob/main/README.md"),
		DownloadURL: github.Ptr("https://raw.githubusercontent.com/owner/repo/main/README.md"),
	}

	expectedFileContent := []mcp.BlobResourceContents{
		{
			Blob: base64.StdEncoding.EncodeToString([]byte("IyBUZXN0IFJlcG9zaXRvcnkKClRoaXMgaXMgYSB0ZXN0IHJlcG9zaXRvcnku")),
		},
	}

	tests := []struct {
		name           string
		mockedClient   *http.Client
		requestArgs    map[string]any
		expectError    bool
		expectedResult any
		expectedErrMsg string
	}{
		{
			name: "successful file content fetch",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposContentsByOwnerByRepoByPath,
					mockFileContent,
				),
			),
			requestArgs: map[string]any{
				"owner":  []string{"owner"},
				"repo":   []string{"repo"},
				"path":   []string{"README.md"},
				"branch": []string{"main"},
			},
			expectError:    false,
			expectedResult: expectedFileContent,
		},
		{
			name: "successful directory content fetch",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposContentsByOwnerByRepoByPath,
					mockDirContent,
				),
			),
			requestArgs: map[string]any{
				"owner": []string{"owner"},
				"repo":  []string{"repo"},
				"path":  []string{"src"},
			},
			expectError:    false,
			expectedResult: expectedDirContent,
		},
		{
			name: "empty content fetch",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposContentsByOwnerByRepoByPath,
					[]*github.RepositoryContent{},
				),
			),
			requestArgs: map[string]any{
				"owner": []string{"owner"},
				"repo":  []string{"repo"},
				"path":  []string{"src"},
			},
			expectError:    false,
			expectedResult: nil,
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
			requestArgs: map[string]any{
				"owner":  []string{"owner"},
				"repo":   []string{"repo"},
				"path":   []string{"nonexistent.md"},
				"branch": []string{"main"},
			},
			expectError:    true,
			expectedErrMsg: "404 Not Found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := github.NewClient(tc.mockedClient)
			handler := repoContentsResourceHandler(client)

			request := mcp.ReadResourceRequest{
				Params: struct {
					URI       string         `json:"uri"`
					Arguments map[string]any `json:"arguments,omitempty"`
				}{
					Arguments: tc.requestArgs,
				},
			}

			resp, err := handler(context.TODO(), request)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
				return
			}

			require.NoError(t, err)
			require.ElementsMatch(t, resp, tc.expectedResult)
		})
	}
}
