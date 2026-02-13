---
description: Save a new version of the project to GitHub
---

This workflow standardizes the process of saving a new version (iteration).

1.  **Determine Next Version**: Check `git tag` for the latest version and increment it (e.g., from `v0.0.9` to `v0.0.10`).
2.  **Update Files**: Update the version string in `frontend/package.json`.
3.  **Commit**:
    ```bash
    git add frontend/package.json
    git commit -m "v<NEW_VERSION>"
    ```
4.  **Tag**:
    ```bash
    git tag v<NEW_VERSION>
    ```
5.  **Push**:
    ```bash
    git push origin main
    git push origin v<NEW_VERSION>
    ```
