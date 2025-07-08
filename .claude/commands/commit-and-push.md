# Quick checks before staging
git status
git diff --cached --check
git log --oneline -3

# Stage everything in current directory
git add .

# Commit using Conventional Commits format
git commit -m "fix(api): handle null user ID on login

- add fallback for missing user context
- log failed auth attempts
- update response error codes"

# Push to upstream branch
git log --oneline -1
git push --set-upstream origin your-branch-name

# Confirm clean working state
git status  # should report: 'working tree clean'

---

**Avoid Commit Loss**

**SAFE:**

```
git status
git log --oneline -5
git stash        # temporary save
git revert       # undo safely
```

**DANGEROUS (avoid unless absolutely necessary):**

```
git reset --hard
git reset HEAD~1
git add .        # stages unintended files
```

---

**Success Criteria**

- Only intended files are staged
- Commit message follows **Conventional Commits** format
- All **pre-commit checks** pass
- git status reports a **clean working tree** after push
