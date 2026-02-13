# Ubuntu Setup Guide (SSH Method)

Documentation shows your `git-remote-https` helper is present but failing to execute (likely a broken system library link). The fastest fix is to **switch to SSH**, which uses a completely different mechanism.

## 1. Switch to SSH Protocol

Run the authentication again, but select **SSH** this time.

```bash
gh auth login
```

1.  **Account**: `GitHub.com`
2.  **Protocol**: Select **SSH** (use arrow keys).
3.  **Upload Public Key**: Select **Generate a new SSH key**.
    *   Enter a passphrase (or allow it to be empty).
    *   Title: `ubuntu-laptop` (or similar).
4.  **Authenticate**: `Login with a web browser`.
    *   Copy the code.
    *   Authorize on your Mac.

## 2. Clone with SSH

Now clone the repo. `gh` will automatically use the `git@github.com...` URL.

```bash
gh repo clone OHMEED/stable-streaming
cd stable-streaming
```

## 3. Verify

If the folder `stable-streaming` appears, you are successful.

```bash
ls -F stable-streaming/
```

## 4. (Optional) Run Diagnosis on Link
If you are curious why HTTPS failed, you can run this to see where the broken link points:
```bash
ls -l /usr/lib/git-core/git-remote-https
```
But you don't need to fix it if existing via SSH works.
