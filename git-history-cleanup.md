# How to Remove Sensitive Data from Git History

If you ever accidentally commit a password, API key, or any sensitive data and push it to GitHub, you need to rewrite your Git history to remove it completely. 

Here is the step-by-step guide on how to do this using `git filter-repo` (the modern and recommended tool for rewriting history).

## Prerequisites

You need to have `git filter-repo` installed. If you don't have it, you can install it using Python's package manager:

```bash
pip install git-filter-repo
```

Or on macOS using Homebrew:
```bash
brew install git-filter-repo
```

## Step 1: Create a replacements file

Create a simple text file (e.g., `replacements.txt`) in the root of your repository. In this file, specify the exact sensitive string you want to remove and what you want to replace it with. 

The format is `exact_string==>replacement_string`.

For example, to replace your password with `********`, write the following inside `replacements.txt`:

```text
MySecretPassword123==>********
```

## Step 2: Run `git filter-repo`

Run the following command to rewrite your Git history. 

*(Note: `git filter-repo` normally expects a fresh clone. Since we are doing this in your current workspace, we use the `--force` flag.)*

```bash
git filter-repo --replace-text replacements.txt --force
```

This command will go through every single commit in your repository and replace the sensitive string with the replacement string.

## Step 3: Restore your remote

By default, as a safety mechanism, `git filter-repo` removes your remote (origin) so you don't accidentally push the rewritten history before verifying it. 

You need to add your remote back:

```bash
git remote add origin https://github.com/your-username/your-repo-name.git
```

## Step 4: Force push to GitHub

Since you have rewritten the history, your local repository's history will diverge from what is on GitHub. You must forcefully overwrite the history on GitHub:

```bash
git push --force origin main
```
*(Change `main` to whatever branch you are on, or use `--all` to push all branches).*

## Step 5: Clean up

Finally, delete the `replacements.txt` file so you don't accidentally commit it!

```bash
rm replacements.txt
```

---

# How to Remove Files from Git History

If you accidentally committed a large binary file, a compiled executable (like `auto-login`), or sensitive files, you can remove the entire file from all commits using the same tool.

```bash
git filter-repo --path filename1 --path filename2 --invert-paths --force
```
*(Replace `filename1` with the exact path of the file you want to remove).*

Just like text replacement, this will remove your remote. You must restore it and force push:

```bash
git remote add origin https://github.com/your-username/your-repo-name.git
git push --force origin main
```

---

### ⚠️ IMPORTANT SECURITY NOTE
If a password or API key was pushed to a public repository on GitHub, **assume it is compromised immediately.** Bots scan public repositories for leaked secrets within seconds. Even after removing it from your history, you **must** change the exposed password or revoke the exposed API key on the corresponding service.
