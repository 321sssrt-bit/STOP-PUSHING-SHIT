package main

import "testing"

func repositoryEvent(t *testing.T) Event {
	t.Helper()
	e := setup(t)
	e.Tool = "mcp__github__create_repository"
	e.Input = map[string]any{"name": "new-project", "private": true, "description": "先审阅，再建仓"}
	return e
}

func TestRepositorySingleUse(t *testing.T) {
	for _, tool := range []string{"mcp__github__create_repository", "github.create_repo", "github.create_repository_from_template", "github.repository_write"} {
		t.Run(tool, func(t *testing.T) {
			e := repositoryEvent(t)
			e.Tool = tool
			e.Input.(map[string]any)["method"] = "create"
			e = grant(t, e)
			if len(call(t, e)) != 0 || state(t, e) != (State{}) {
				t.Fatal("repository grant must be consumed before execution")
			}
			blocked(t, call(t, e))
		})
	}
	t.Run("fork", func(t *testing.T) {
		e := setup(t)
		e.Tool = "github.fork_repository"
		e.Input = map[string]any{"owner": "example", "repo": "source", "organization": "destination"}
		e = grant(t, e)
		if len(call(t, e)) != 0 {
			t.Fatal("fork grant did not work")
		}
		blocked(t, call(t, e))
	})
}

func TestRepositoryBinding(t *testing.T) {
	for field, value := range map[string]any{"name": "other-project", "private": false, "organization": "other-org", "description": "changed", "autoInit": true, "template_repo": "other-template"} {
		t.Run(field, func(t *testing.T) {
			e := grant(t, repositoryEvent(t))
			id := state(t, e).Request
			e.Input.(map[string]any)[field] = value
			blocked(t, call(t, e))
			if state(t, e).Request == id {
				t.Fatal("changed repository request kept its grant")
			}
		})
	}
	for field, value := range map[string]any{"name": " ", "private": "true"} {
		t.Run("invalid "+field, func(t *testing.T) {
			e := grant(t, repositoryEvent(t))
			original := e.Input.(map[string]any)[field]
			e.Input.(map[string]any)[field] = value
			blocked(t, call(t, e))
			if state(t, e) != (State{}) {
				t.Fatal("incomplete request retained authorization")
			}
			e.Input.(map[string]any)[field] = original
			blocked(t, call(t, e))
		})
	}
	t.Run("explicit visibility", func(t *testing.T) {
		e := repositoryEvent(t)
		delete(e.Input.(map[string]any), "private")
		blocked(t, call(t, e))
		if state(t, e).Request != "" {
			t.Fatal("missing visibility must not be authorizable")
		}
	})
	t.Run("public requires its own grant", func(t *testing.T) {
		e := repositoryEvent(t)
		e.Input.(map[string]any)["private"] = false
		e = grant(t, e)
		if len(call(t, e)) != 0 {
			t.Fatal("explicitly approved public request did not work")
		}
	})
}

func TestRepositoryCommands(t *testing.T) {
	for _, tc := range []struct {
		source string
		deny   bool
	}{
		{`gh repo create`, true},
		{`gh repo --help=false create project --private`, true},
		{`gh repo new example/project --private`, true},
		{`gh repo create --private --source=. --push`, true},
		{`gh repo create example/project --template example/template --public`, true},
		{`gh repo fork example/project --clone=false`, true},
		{`gh repo create --description --help`, true},
		{`gh repo create -h https://example.com --private`, true},
		{`gh repo fork --org --help`, true},
		{`gh api user/repos -f name=project`, true},
		{`gh api orgs/example/repos --method POST -f name=project`, true},
		{`curl --json '{"name":"project"}' https://api.github.com/user/repos`, true},
		{`curl -X POST https://enterprise.example/api/v3/orgs/example/repos`, true},
		{`gh api repos/example/template/generate -f name=project`, true},
		{`gh api repos/example/project/forks -X POST`, true},
		{`gh api graphql -f 'query=mutation { createRepository(input: {}) { repository { id } } }'`, true},
		{`gh api graphql -f 'query=mutation { cloneTemplateRepository(input: {}) { repository { id } } }'`, true},
		{`python3 -c 'import subprocess; subprocess.run(["gh","repo","create","project","--private"])'`, true},
		{`python3 -c 'import requests; requests.post("https://api.github.com/user/repos",json={"name":"project"})'`, true},
		{`python3 -c 'client.graphql("mutation { createRepository(input: {}) { repository { id } } }")'`, true},
		{`pwsh -Command 'gh.exe repo create project --private'`, true},
		{`Invoke-RestMethod -Method POST -Uri https://api.github.com/orgs/example/repos -Body '{}'`, true},
		{`cmd.exe /c "gh.exe repo fork example/project"`, true},
		{`gh repo create --help`, false},
		{`gh repo --help`, false},
		{`gh repo create project --help`, false},
		{`gh repo fork example/project -h`, false},
		{`gh repo view example/project`, false},
		{`gh repo list example`, false},
		{`gh repo clone example/project`, false},
		{`gh api user/repos`, false},
		{`gh api orgs/example/repos -X GET`, false},
		{`curl -I https://api.github.com/repos/example/project/forks`, false},
		{`Invoke-RestMethod -Method GET -Uri https://api.github.com/user/repos`, false},
		{`python3 -c 'import requests; requests.get("https://api.github.com/user/repos")'`, false},
		{`git init`, false},
		{`git clone https://github.com/example/project.git`, false},
		{`printf '%s' 'gh repo create project'`, false},
		{`gh api graphql -f 'query=query { viewer { repositories(first: 10) { nodes { name } } } }'`, false},
		{`gh api graphql -f 'query=mutation { createRepositoryRuleset(input: {}) { clientMutationId } }'`, false},
	} {
		t.Run(tc.source, func(t *testing.T) {
			if got := shellCreation(tc.source, 0, ""); got != tc.deny {
				t.Fatalf("deny=%v want %v", got, tc.deny)
			}
		})
	}
}
