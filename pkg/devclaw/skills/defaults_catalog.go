// Package skills – defaults_catalog.go holds the embedded default skill templates.
//
// Only core development/coding skills belong here. These are the "Starter Pack"
// — pre-selected during setup and always available offline.
//
// Integration, productivity, infra, and other skills live in the devclaw-skills
// repository and are fetched/installed separately.
//
// Skills teach the LLM how to use existing native tools (bash, ssh, scp,
// read_file, write_file, etc.) — they do NOT register new tools.
package skills

// nolint: lll
var defaultSkillList = []DefaultSkill{
	{
		Name:        "docker",
		Label:       "🐳 Docker — containers, images, compose",
		Description: "Manage Docker containers, images, volumes and Compose",
		Category:    "development",
		StarterPack: true,
		Content: `---
name: docker
description: "Manage Docker containers, images, volumes and Compose"
---
# Docker

Use the **bash** tool to manage Docker containers, images, and services.

## Containers
` + "```" + `bash
docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}"
docker ps -a --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
docker logs <container> --tail 100
docker exec -it <container> <command>
docker stop <container>
docker restart <container>
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}"
docker inspect <container> --format '{{json .Config.Env}}'
` + "```" + `

## Images
` + "```" + `bash
docker images --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
docker build -t <tag> .
docker push <image>:<tag>
docker pull <image>:<tag>
` + "```" + `

## Docker Compose
` + "```" + `bash
docker compose up -d
docker compose down
docker compose ps
docker compose logs <service> --tail 50
docker compose build --no-cache
docker compose up -d --build
` + "```" + `

## Cleanup
` + "```" + `bash
docker system prune -f
docker volume prune -f
docker system df
` + "```" + `

## Tips
- Always use --tail in logs to avoid huge output
- Use --format with Go templates for clean output
- Use docker compose (v2) instead of docker-compose (v1)
`,
	},
	{
		Name:        "git-advanced",
		Label:       "🌳 Git — rebase, bisect, stash, cherry-pick",
		Description: "Advanced Git: rebase, stash, bisect, cherry-pick, worktree",
		Category:    "development",
		StarterPack: true,
		Content: `---
name: git-advanced
description: "Advanced Git: rebase, bisect, stash, cherry-pick, worktree"
---
# Git Advanced

Use the **bash** tool for advanced Git operations.

## Status & Diff
` + "```" + `bash
git status --porcelain=v2 -b
git diff --stat
git diff --staged --stat
git log --oneline --graph -20
` + "```" + `

## Rebase
` + "```" + `bash
git rebase -i HEAD~<N>
git rebase main
git rebase --continue
git rebase --abort
` + "```" + `

## Stash
` + "```" + `bash
git stash push -m "description"
git stash list
git stash pop
git stash apply stash@{N}
git stash drop stash@{N}
` + "```" + `

## Cherry-pick
` + "```" + `bash
git cherry-pick <hash>
git cherry-pick --no-commit <hash>
git cherry-pick --abort
` + "```" + `

## Bisect
` + "```" + `bash
git bisect start
git bisect bad
git bisect good <hash>
git bisect good   # or git bisect bad
git bisect reset
` + "```" + `

## Log & Search
` + "```" + `bash
git log --oneline --graph --all
git log --grep="keyword"
git log -S "functionName"
git log --author="name" --since="1 week ago"
` + "```" + `

## Worktree & Cleanup
` + "```" + `bash
git worktree add ../feature feature-branch
git worktree list
git branch --merged main | grep -v main | xargs -r git branch -d
git fetch --prune
git reflog
` + "```" + `

## Tips
- Use git reflog to recover from any mistake
- Never rebase public/shared branches
- Use --porcelain for machine-readable output
`,
	},
	{
		Name:        "npm",
		Label:       "📦 npm/yarn/pnpm — Node.js packages",
		Description: "Manage Node.js packages and scripts with npm, yarn, pnpm",
		Category:    "development",
		StarterPack: true,
		Content: `---
name: npm
description: "Manage Node.js packages and scripts with npm, yarn, pnpm"
---
# npm / yarn / pnpm

Use the **bash** tool to manage Node.js dependencies and scripts.

## Install
` + "```" + `bash
npm install
npm install <pkg> --save-dev
yarn add <pkg> --dev
pnpm add <pkg> -D
` + "```" + `

## Scripts
` + "```" + `bash
cat package.json | jq '.scripts'
npm run <script>
npm run build
npm run dev
npm test
` + "```" + `

## Audit & Update
` + "```" + `bash
npm audit
npm audit fix
npm outdated
npm update <pkg>
` + "```" + `

## Info
` + "```" + `bash
npm ls --depth=0
npm info <pkg> versions
npx <pkg> [args]
` + "```" + `

## Tips
- Detect package manager by lockfile: package-lock.json (npm), yarn.lock (yarn), pnpm-lock.yaml (pnpm)
- Use npm ci in CI/CD (faster, respects lockfile)
- Use npx to run packages without global install
`,
	},
	{
		Name:        "go-tools",
		Label:       "🐹 Go — build, test, vet, mod",
		Description: "Go development: build, test, vet, modules, vulnerability checks",
		Category:    "development",
		StarterPack: true,
		Content: `---
name: go-tools
description: "Go development: build, test, vet, modules, vulnerability checks"
---
# Go Tools

Use the **bash** tool for Go development workflows.

## Build & Run
` + "```" + `bash
go build ./...
go build -o <binary> ./cmd/<main>
GOOS=linux GOARCH=amd64 go build -o <binary>-linux-amd64 ./cmd/<main>
go run ./cmd/<main>
` + "```" + `

## Test
` + "```" + `bash
go test ./...
go test -v -run TestName ./pkg/<package>/
go test -cover ./...
go test -race ./...
go test -bench=. ./pkg/<package>/
` + "```" + `

## Lint & Vet
` + "```" + `bash
go vet ./...
staticcheck ./...
golangci-lint run
gofmt -w .
` + "```" + `

## Modules
` + "```" + `bash
go mod tidy
go mod verify
go get <pkg>@<version>
go list -m all
` + "```" + `

## Vulnerabilities
` + "```" + `bash
govulncheck ./...
` + "```" + `

## Tips
- Always run go vet before commit
- Use -race in tests to detect data races
- Cross-compile with GOOS and GOARCH
`,
	},
	{
		Name:        "ssh-tools",
		Label:       "🔑 SSH — keys, tunnels, remote access",
		Description: "SSH key management, connections, SCP, rsync, port forwarding",
		Category:    "development",
		StarterPack: true,
		Content: `---
name: ssh-tools
description: "SSH key management, connections, SCP, rsync, port forwarding"
---
# SSH Tools

Use **bash**, **ssh**, and **scp** native tools for remote operations.

## Keys
` + "```" + `bash
ssh-keygen -t ed25519 -C "email@example.com"
ssh-copy-id -i ~/.ssh/<key>.pub <user>@<host>
ssh-keygen -l -f ~/.ssh/<key>.pub
` + "```" + `

## Connect & Execute
Use the **ssh** tool directly, or **bash** for local SSH:
` + "```" + `bash
ssh <user>@<host> "cd /app && docker ps"
ssh -J bastion user@target
` + "```" + `

## File Transfer
Use the **scp** tool, or **bash** for rsync:
` + "```" + `bash
scp <file> <user>@<host>:<path>
rsync -avz --progress <local>/ <user>@<host>:<remote>/
` + "```" + `

## Port Forwarding
` + "```" + `bash
ssh -L 5432:localhost:5432 <user>@<host>   # local forward
ssh -R 8080:localhost:3000 <user>@<host>   # remote forward
ssh -D 1080 <user>@<host>                  # SOCKS proxy
` + "```" + `

## Tips
- Prefer Ed25519 over RSA
- Use SSH config for aliases: read_file ~/.ssh/config
- Use edit_file to modify SSH config
`,
	},
	{
		Name:        "github",
		Label:       "🐙 GitHub — PRs, issues, actions via gh CLI",
		Description: "GitHub integration via gh CLI: issues, PRs, releases, CI",
		Category:    "development",
		StarterPack: true,
		Content: `---
name: github
description: "GitHub integration via gh CLI"
---
# GitHub

Use the **bash** tool with the gh CLI for GitHub operations.

## Repos & PRs
` + "```" + `bash
gh repo list --limit 10
gh pr list -R OWNER/REPO --limit 10
gh pr create -R OWNER/REPO --title "TITLE" --body "BODY"
gh pr merge NUMBER -R OWNER/REPO --squash
gh pr view NUMBER -R OWNER/REPO
` + "```" + `

## Issues
` + "```" + `bash
gh issue list -R OWNER/REPO --limit 10
gh issue create -R OWNER/REPO --title "TITLE" --body "BODY"
gh issue close NUMBER -R OWNER/REPO
` + "```" + `

## Actions & Releases
` + "```" + `bash
gh run list -R OWNER/REPO --limit 5
gh run view RUN_ID -R OWNER/REPO --log
gh release list -R OWNER/REPO --limit 5
gh release create TAG -R OWNER/REPO --title "TITLE" --notes "NOTES"
` + "```" + `

## Tips
- Use -R OWNER/REPO to target a specific repo
- Use --json for structured output (pipe to jq)
- Check auth: gh auth status
`,
	},
}
