package github

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/go-github-mock/github-mock/mock"
	"github.com/mrinjamul/mcp-go"
	"github.com/stretchr/testify/assert"
)

func TestGetFileContents(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	t.Logf("current working directory: %s", wd)

	// test get_file_contents
	t.Run("get_file_contents", func(t *testing.T) {
		t.Parallel()
		// test with valid file path
		t.Run("with valid file path", func(t *testing.T) {
			t.Parallel()
			mockedHTTPClient := mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposContentsByOwnerByRepoByPath,
					mock.WithPath("README.md"),
					mock.WithHeader("Accept", "application/vnd.github.v3.raw"),
				),
				mock.WithResponse(
					mock.WithStatusCode(200),
					mock.WithBody([]byte("test content")),
				),
			)
			g, err := newTestGithub(
				context.Background(),
				"test-token",
				mockedHTTPClient,
			)
			assert.NoError(t, err)

			resp, err := g.GetFileContents(context.Background(), &GetFileContentsArgs{
				Owner: "test-owner",
				Repo:  "test-repo",
				Path:  "README.md",
			})
			assert.NoError(t, err)
			assert.Equal(t, "test content", resp.Content)
		})
		// test with invalid file path
		t.Run("with invalid file path", func(t *testing.T) {
			t.Parallel()
			mockedHTTPClient := mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposContentsByOwnerByRepoByPath,
					mock.WithPath("not-found.md"),
				),
				mock.WithResponse(
					mock.WithStatusCode(404),
				),
			)
			g, err := newTestGithub(
				context.Background(),
				"test-token",
				mockedHTTPClient,
			)
			assert.NoError(t, err)

			_, err = g.GetFileContents(context.Background(), &GetFileContentsArgs{
				Owner: "test-owner",
				Repo:  "test-repo",
				Path:  "not-found.md",
			})
			assert.Error(t, err)
		})
		// test with directory path
		t.Run("with directory path", func(t *testing.T) {
			t.Parallel()
			mockedHTTPClient := mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposContentsByOwnerByRepoByPath,
					mock.WithPath("test-dir/"),
				),
				mock.WithResponse(
					mock.WithStatusCode(200),
					mock.WithBody([]byte(`[{"type": "file", "name": "test-file.md"}]`)),
				),
			)
			g, err := newTestGithub(
				context.Background(),
				"test-token",
				mockedHTTPClient,
			)
			assert.NoError(t, err)

			resp, err := g.GetFileContents(context.Background(), &GetFileContentsArgs{
				Owner: "test-owner",
				Repo:  "test-repo",
				Path:  "test-dir/",
			})
			assert.NoError(t, err)
			var data []map[string]interface{}
			err = json.Unmarshal([]byte(resp.Content), &data)
			assert.NoError(t, err)
			assert.Equal(t, "file", data[0]["type"])
			assert.Equal(t, "test-file.md", data[0]["name"])
		})
		// test with fallback
		t.Run("with fallback", func(t *testing.T) {
			t.Parallel()
			mockedHTTPClient := mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposContentsByOwnerByRepoByPath,
					mock.WithPath("README.md"),
					mock.WithHeader("Accept", "application/vnd.github.v3+json"),
				),
				mock.WithResponse(
					mock.WithStatusCode(404),
				),
				mock.WithRequestMatch(
					mock.GetReposContentsByOwnerByRepoByPath,
					mock.WithPath("README.md"),
					mock.WithHeader("Accept", "application/vnd.github.v3.raw"),
				),
				mock.WithResponse(
					mock.WithStatusCode(200),
					mock.WithBody([]byte("test content from fallback")),
				),
			)

			g, err := newTestGithub(
				context.Background(),
				"test-token",
				mockedHTTPClient,
			)
			assert.NoError(t, err)

			resp, err := g.GetFileContents(context.Background(), &GetFileContentsArgs{
				Owner:            "test-owner",
				Repo:             "test-repo",
				Path:             "README.md",
				AllowRawFallback: boolPtr(true),
			})
			assert.NoError(t, err)
			assert.Equal(t, "test content from fallback", resp.Content)
		})
		// test without fallback
		t.Run("without fallback", func(t *testing.T) {
			t.Parallel()
			mockedHTTPClient := mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposContentsByOwnerByRepoByPath,
					mock.WithPath("README.md"),
					mock.WithHeader("Accept", "application/vnd.github.v3+json"),
				),
				mock.WithResponse(
					mock.WithStatusCode(404),
				),
			)

			g, err := newTestGithub(
				context.Background(),
				"test-token",
				mockedHTTPClient,
			)
			assert.NoError(t, err)

			_, err = g.GetFileContents(context.Background(), &GetFileContentsArgs{
				Owner:            "test-owner",
				Repo:             "test-repo",
				Path:             "README.md",
				AllowRawFallback: boolPtr(false),
			})
			assert.Error(t, err)
			toolErr, ok := err.(*mcp.ToolResultError)
			assert.True(t, ok)
			assert.Contains(t, toolErr.Error(), "status: 404")
		})
		// Test with SHA and no fallback
		t.Run("with SHA and no fallback", func(t *testing.T) {
			t.Parallel()
			mockedHTTPClient := mock.NewMockedHTTPClient(
				mock.WithRequestMatch(
					mock.GetReposContentsByOwnerByRepoByPath,
					mock.WithPath("README.md"),
					mock.WithHeader("Accept", "application/vnd.github.v3+json"),
				),
				mock.WithResponse(
					mock.WithStatusCode(200),
					mock.WithBody([]byte(`{"sha": "test-sha", "content": "dGVzdCBjb250ZW50"}`)), // base64 of "test content"
				),
			)
			g, err := newTestGithub(
				context.Background(),
				"test-token",
				mockedHTTPClient,
			)
			assert.NoError(t, err)

			resp, err := g.GetFileContents(context.Background(), &GetFileContentsArgs{
				Owner:            "test-owner",
				Repo:             "test-repo",
				Path:             "README.md",
				AllowRawFallback: boolPtr(false),
			})
			assert.NoError(t, err)
			assert.Equal(t, "test-sha", resp.SHA)
			assert.Equal(t, "test content", resp.Content)
		})
	})
}

// Helper to get a pointer to a boolean value
func boolPtr(b bool) *bool {
	return &b
}
