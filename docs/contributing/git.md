## Git best-practices guide

Write good commit messages.
- https://chris.beams.io/posts/git-commit/
- https://www.gitkraken.com/learn/git/best-practices/git-commit-message

[Rebase, don't merge](https://www.atlassian.com/git/tutorials/rewriting-history/git-rebase).


Configure your fork to rebase from the upstream master branch by default:
```bash 
git config branch.master.remote upstream && git config branch.autoSetupRebase always
```
