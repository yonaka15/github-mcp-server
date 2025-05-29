package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v69/github"
	"github.com/sammorrowdrums/mcp-go/mcp"
)

// RepositoryResourceCompletionHandler returns a CompletionHandlerFunc for repository resource completions.
func RepositoryResourceCompletionHandler(getClient GetClientFn) func(ctx context.Context, req mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	return func(ctx context.Context, req mcp.CompleteRequest) (*mcp.CompleteResult, error) {
		ref, ok := req.Params.Ref.(map[string]any)
		if !ok || ref["type"] != "ref/resource" {
			return nil, nil // Not a resource completion
		}
		uri, _ := ref["uri"].(string)
		argName := req.Params.Argument.Name
		argValue := req.Params.Argument.Value

		client, err := getClient(ctx)
		if err != nil {
			return nil, err
		}

		var values []string

		switch argName {
		case "owner":
			user, _, err := client.Users.Get(ctx, "")
			if err == nil && user.GetLogin() != "" {
				values = append(values, user.GetLogin())
			}
			orgs, _, _ := client.Organizations.List(ctx, "", nil)
			for _, org := range orgs {
				values = append(values, org.GetLogin())
			}
		case "repo":
			// print the whole mcp complete request for debugging
			fmt.Printf("MCP Complete Request: %+v\n", req)

			fmt.Printf("URI: %s\n", uri)
			owner := getArgFromURI(uri, "owner")
			if owner != "" {
				repos, _, err := client.Search.Repositories(ctx, fmt.Sprintf("user:%s", owner), &github.SearchOptions{ListOptions: github.ListOptions{PerPage: 100}})
				if err != nil || repos == nil {
					break
				}
				for _, repo := range repos.Repositories {
					if argValue == "" || strings.Contains(repo.GetName(), argValue) {
						values = append(values, repo.GetName())
					}
				}
			}
		case "branch":
			owner := getArgFromURI(uri, "owner")
			repo := getArgFromURI(uri, "repo")
			if owner != "" && repo != "" {
				branches, _, _ := client.Repositories.ListBranches(ctx, owner, repo, nil)
				for _, branch := range branches {
					if argValue == "" || strings.Contains(branch.GetName(), argValue) {
						values = append(values, branch.GetName())
					}
				}
			}
		case "sha":
			owner := getArgFromURI(uri, "owner")
			repo := getArgFromURI(uri, "repo")
			if owner != "" && repo != "" {
				commits, _, _ := client.Repositories.ListCommits(ctx, owner, repo, nil)
				for _, commit := range commits {
					sha := commit.GetSHA()
					if argValue == "" || strings.HasPrefix(sha, argValue) {
						values = append(values, sha)
					}
				}
			}
		case "tag":
			owner := getArgFromURI(uri, "owner")
			repo := getArgFromURI(uri, "repo")
			if owner != "" && repo != "" {
				tags, _, _ := client.Repositories.ListTags(ctx, owner, repo, nil)
				for _, tag := range tags {
					if argValue == "" || strings.Contains(tag.GetName(), argValue) {
						values = append(values, tag.GetName())
					}
				}
			}
		case "prNumber":
			owner := getArgFromURI(uri, "owner")
			repo := getArgFromURI(uri, "repo")
			if owner != "" && repo != "" {
				prs, _, _ := client.PullRequests.List(ctx, owner, repo, nil)
				for _, pr := range prs {
					num := fmt.Sprintf("%d", pr.GetNumber())
					if argValue == "" || strings.HasPrefix(num, argValue) {
						values = append(values, num)
					}
				}
			}
		case "path":
			owner := getArgFromURI(uri, "owner")
			repo := getArgFromURI(uri, "repo")
			refVal := getArgFromURI(uri, "branch")
			if refVal == "" {
				refVal = getArgFromURI(uri, "sha")
			}
			if refVal == "" {
				refVal = getArgFromURI(uri, "tag")
			}
			if refVal == "" {
				refVal = "main"
			}
			if owner != "" && repo != "" {
				contents, dirContents, _, _ := client.Repositories.GetContents(ctx, owner, repo, "", &github.RepositoryContentGetOptions{Ref: refVal})
				if dirContents != nil {
					for _, entry := range dirContents {
						if argValue == "" || strings.HasPrefix(entry.GetName(), argValue) {
							values = append(values, entry.GetName())
						}
					}
				} else if contents != nil {
					if argValue == "" || strings.HasPrefix(contents.GetName(), argValue) {
						values = append(values, contents.GetName())
					}
				}
			}
		}

		if len(values) > 100 {
			values = values[:100]
		}

		return &mcp.CompleteResult{
			Completion: struct {
				Values  []string `json:"values"`
				Total   int      `json:"total,omitempty"`
				HasMore bool     `json:"hasMore,omitempty"`
			}{
				Values:  values,
				Total:   len(values),
				HasMore: false,
			},
		}, nil
	}
}

func getArgFromURI(uri, name string) string {
	trimmed := strings.TrimPrefix(uri, "repo://")
	parts := strings.Split(trimmed, "/")
	if name == "owner" && len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	if name == "repo" && len(parts) > 1 && parts[1] != "" {
		return parts[1]
	}
	return ""
}
