**Create GitHub Issues for SA AI Assitant (via gh CLI)**

---

**🔹 Purpose**

Create a GitHub issue for task **#$ARGUMENTS** aligned with our platform architecture.

---

**🧩 Issue Types & Templates**

Use the following structure and safe label combinations:

| **Type** | **Command Example** | **Labels** |
| --- | --- | --- |
| New Feature / Enhancement | gh issue create --title "SA AI Assitant: Enhancement – $DESCRIPTION" --label "enhancement" --assignee @me | enhancement |
| Technical Debt / Standards | gh issue create --title "SA AI Assitant: Standards – $DESCRIPTION" --label "standards,technical-debt" --assignee @me | standards, technical-debt |
| Security Vulnerability | gh issue create --title "SA AI Assitant: Security – $DESCRIPTION" --label "security,critical" --assignee @me | security, critical |
| Bug Report | gh issue create --title "SA AI Assitant: Bug – $DESCRIPTION" --label "bug" --assignee @me | bug |
| Documentation | gh issue create --title "CSA AI Assitant: Docs – $DESCRIPTION" --label "documentation" --assignee @me | documentation |
| Dependency Upgrade | gh issue create --title "CSA AI Assitant: Dependencies – $DESCRIPTION" --label "dependencies,security" --assignee @me | dependencies, security |

---

**✅ Allowed Labels (use only these):**

bug, enhancement, documentation, standards, technical-debt, security, critical, dependencies, tool-migration, infra, devops

**Safe label combinations include:**

- standards, technical-debt
- security, critical, dependencies
- enhancement, tool-migration
- documentation, standards
- devops, technical-debt
- bug, critical, standards

---

**📋 Issue Structure**

1. **Title**

    SA AI Assitant: [Type] – [Short Description]

    *Example:* SA AI Assitant: Bug – Diff CLI crashes on large payload

2. **Summary**
    - What the issue is
    - Why it matters
    - Include any test requirements from the task definition
3. **Problems & Work Plan**
    - List affected files or components (e.g., src/cli/diff.go)
    - Step-by-step implementation plan (including test and demo coverage)
4. **Acceptance Criteria**
    - Measurable success (e.g., “CLI handles 10 MB payload without crash”)
    - Tests implemented and passing
    - Docs updated if applicable
5. **References**
    - ADRs, API specs, related issues, or linked docs

---

**⚙️ Quick Issue Creation Command**

Once title and labels are finalized, run:

```
gh issue create --title "$TITLE" --label "$LABELS" --assignee @me
```

Tip: run gh label list to verify valid labels before creating.

---

**⚠️ Note:**

This is for **issue creation only** — **do not make any code changes yet.**
