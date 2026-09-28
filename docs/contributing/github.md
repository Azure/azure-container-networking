## GitHub contributing guidelines
### General
- Use a [forking workflow](https://www.atlassian.com/git/tutorials/comparing-workflows/forking-workflow). [(How to fork)](https://help.github.com/articles/fork-a-repo/).

### Pull Requests
Do: 
- Scope PRs to the smallest self-contained change possible.
- Open one PR per change (for example: a feature addition and a refactor of related code are separate changes warranting separate PRs).
- Use a descriptive title and description. Provide enough context that anyone familiar with the codebase can understand the purpose of the change and review it.
- Include a link to a related issue if applicable. 
- Attach labels. 
- Assign reviewers if the automated reviewer assignment is insufficient.
- Open PRs early as a draft and add the `WIP` label to get early feedback. 
- Rebase from master when your branch/PR is out of date.
- Fix review feedback in individual commits with descriptive commit messages (and link these commits in the comments).

Do not:
- Link to private issue trackers or internal documents.
- Ask for final reviews unless CI is passing.
- Resolve reviewer comments yourself (allow the reviewer to do so once they are satisfied that their feedback is addressed).
- Close and open a new PR to address git conflicts or feedback (PRs are documentation. One PR per change).

### Pipeline configuration checks

Run `go test -mod=readonly -buildvcs=false -tags unit ./test/pipelines` from the repository root when changing Swift provisioning, Windows overlay tests, the Manifold trigger, or Ubuntu build package sources. These tests check template parameter propagation, stateful defaults, Windows validation, boot cleanup ordering and datapath timeouts, candidate image and region selection, and Azure Ubuntu mirror configuration before package updates without creating clusters or queueing pipelines. They are also included in `make test-main` and `make test-all`.
