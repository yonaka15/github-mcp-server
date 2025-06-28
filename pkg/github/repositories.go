package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-github/v62/github"
	"github.com/kinbiko/jsonassert"
	"github.com/muno92/go-github-mock/mock"
	"github.com/yonaka/mcp-go"
	"github.com/yonaka/mcp-go/pkg/tool"
)

var (
	// Repositories toolset
	RepositoriesToolset = tool.Toolset{
		Name: "repositories",
		Tools: []tool.Tool{
			GetFileContentsTool,
		},
	}
	GetFileContentsTool = tool.Tool{
		Name: "get_file_contents",
		Description: `Get the contents of a file or directory from a GitHub repository.
If the path points to a file, the response will contain the file content.
If the path points to a directory, the response will contain a list of files and directories in that directory.`,
		Run: getFileContents,
		Arguments: []tool.Argument{
			tool.NewRequiredParam("owner", "Repository owner (username or organization)"),
			tool.NewRequiredParam("repo", "Repository name"),
			tool.NewRequiredParam("path", "Path to file/directory"),
			tool.NewOptionalParam("ref", "The name of the commit/branch/tag. Default: the repository’s default branch."),
			// Note: This is a string because we can't unmarshal bools from tool call args yet.
			tool.NewOptionalParam("allow_raw_fallback", "To ensure the file SHA is returned and prevent fallback to raw content, set this to 'false'. Defaults to true."),
		},
	}
)

type GetFileContentsArgs struct {
	Owner            string `json:"owner"`
	Repo             string `json:"repo"`
	Path             string `json:"path"`
	Ref              string `json:"ref,omitempty"`
	AllowRawFallback string `json:"allow_raw_fallback,omitempty"`
}

func getFileContents(ctx context.Context, s tool.ToolState, r mcp.ToolRequest) (mcp.ToolResult, error) {
	var args GetFileContentsArgs
	if err := json.Unmarshal(r.Arguments, &args); err != nil {
		return mcp.ToolResult{}, mcp.NewToolResultError(fmt.Sprintf("Failed to unmarshal arguments: %s", err))
	}

	client := FromContext(ctx)

	fileContent, dirContent, resp, err := client.Repositories.GetContents(
		ctx,
		args.Owner,
		args.Repo,
		args.Path,
		&github.RepositoryContentGetOptions{
			Ref: args.Ref,
		},
	)

	// The GitHub API returns a 404 when the file is too large to be returned.
	// In this case, we try to fetch the raw content of the file.
	// The raw content does not include the SHA, so we should only do this if the user
	// has not explicitly disabled it.
	allowFallback := true
	if args.AllowRawFallback == "false" {
		allowFallback = false
	}

	if err != nil && resp != nil && resp.StatusCode == http.StatusNotFound && allowFallback {
		fileContent, err = getRawContents(ctx, client, args)
		if err != nil {
			return mcp.ToolResult{}, mcp.NewToolResultError(fmt.Sprintf("Failed to get raw contents: %s", err))
		}
	} else if err != nil {
		return mcp.ToolResult{}, mcp.NewToolResultError(fmt.Sprintf("Failed to get contents: %s", err))
	}

	if fileContent != nil {
		content, err := fileContent.GetContent()
		if err != nil {
			return mcp.ToolResult{}, mcp.NewToolResultError(fmt.Sprintf("Failed to get content: %s", err))
		}
		return mcp.ToolResult{
			Result: map[string]interface{}{
				"content": content,
				"sha":     fileContent.GetSHA(),
			},
		}, nil
	}

	var dirEntries []map[string]interface{}
	for _, entry := range dirContent {
		dirEntries = append(dirEntries, map[string]interface{}{
			"name": entry.GetName(),
			"path": entry.GetPath(),
			"sha":  entry.GetSHA(),
			"type": entry.GetType(),
		})
	}

	return mcp.ToolResult{
		Result: map[string]interface{}{
			"content": dirEntries,
		},
	}, nil
}

// getRawContents fetches the raw content of a file from the GitHub API.
// This is used as a fallback when the file is too large to be fetched
// using the GetContents API.
func getRawContents(ctx context.Context, client *github.Client, args GetFileContentsArgs) (*github.RepositoryContent, error) {
	// We build a request to the raw content endpoint.
	// The go-github library doesn't have a dedicated method for this, so we construct the URL manually.
	path := fmt.Sprintf("repos/%s/%s/contents/%s", args.Owner, args.Repo, args.Path)

	req, err := client.NewRequest(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set the query parameter for the ref, if provided.
	if args.Ref != "" {
		q := req.URL.Query()
		q.Set("ref", args.Ref)
		req.URL.RawQuery = q.Encode()
	}

	// We need to set the Accept header to get the raw content.
	req.Header.Set("Accept", "application/vnd.github.raw")

	var buf io.ReadCloser
	resp, err := client.Do(ctx, req, &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to get raw content: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get raw content: status code %d, body: %s", resp.StatusCode, string(body))
	}
	defer buf.Close()

	b, err := io.ReadAll(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read raw content: %w", err)
	}

	return &github.RepositoryContent{
		Content: github.String(base64.StdEncoding.EncodeToString(b)),
	}, nil
}

// Mock helpers for testing

// MockGetFileContentsSuccess mocks a successful call to the GetFileContents tool.
func MockGetFileContentsSuccess(
	t *testing.T,
	mockedHTTPClient *http.ServeMux,
	owner, repo, path string,
	options *github.RepositoryContentGetOptions,
	response *github.RepositoryContent,
) {
	t.Helper()

	mock.EndpointPattern(
		mockedHTTPClient,
		fmt.Sprintf("GET /repos/%s/%s/contents/%s", owner, repo, path),
	).
		WithQuery(
			"ref",
			options.GetRef(),
		).
		Response(
			http.StatusOK,
			mock.MustMarshal(response),
		)
}

// MockGetDirContentsSuccess mocks a successful call to the GetFileContents tool
// when the path is a directory.
func MockGetDirContentsSuccess(
	t *testing.T,
	mockedHTTPClient *http.ServeMux,
	owner, repo, path string,
	options *github.RepositoryContentGetOptions,
	response []*github.RepositoryContent,
) {
	t.Helper()

	mock.EndpointPattern(
		mockedHTTPClient,
		fmt.Sprintf("GET /repos/%s/%s/contents/%s", owner, repo, path),
	).
		WithQuery(
			"ref",
			options.GetRef(),
		).
		Response(
			http.StatusOK,
			mock.MustMarshal(response),
		)
}

// MockGetContentsNotFound mocks a call to the GetFileContents tool that
// returns a 404 Not Found error.
func MockGetContentsNotFound(
	t *testing.T,
	mockedHTTPClient *http.ServeMux,
	owner, repo, path string,
	options *github.RepositoryContentGetOptions,
) {
	t.Helper()

	mock.EndpointPattern(
		mockedHTTPClient,
		fmt.Sprintf("GET /repos/%s/%s/contents/%s", owner, repo, path),
	).
		WithQuery(
			"ref",
			options.GetRef(),
		).
		Response(
			http.StatusNotFound,
			mock.MustMarshal(github.ErrorResponse{
				Message: "Not Found",
			}),
		)
}

func expectQueryParams(t *testing.T, expected map[string]string) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		t.Helper()

		ja := jsonassert.New(t)
		q := r.URL.Query()
		for k, v := range expected {
			ja.Assertf(q.Get(k), v, "expected query param %s to be %s", k, v)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mock.MustMarshal([]*github.RepositoryContent{}))
	}
}

// MockGetRawContentsSuccess mocks a successful call to get the raw content of a
// file.
func MockGetRawContentsSuccess(
	t *testing.T,
	mockedHTTPClient *http.ServeMux,
	owner, repo, ref, path string,
	response string,
) {
	t.Helper()

	pattern := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, path)

	mockedHTTPClient.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github.raw" {
			// Not a raw request, so we don't handle it.
			w.WriteHeader(http.StatusNotImplemented)
			return
		}

		if ref != "" && r.URL.Query().Get("ref") != ref {
			t.Errorf("expected ref to be %s, got %s", ref, r.URL.Query().Get("ref"))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(response))
	})
}
